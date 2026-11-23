package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/auth"
	"github.com/xiaoyueRX/Ani-Go/internal/database"
)

func bindReviewedFile(t *testing.T, ep *database.Episode, kind string) string {
	t.Helper()
	ep.DownloadRoot = t.TempDir()
	path := filepath.Join(ep.DownloadRoot, ep.SourceFile)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test media"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ep.FileSize = st.Size()
	ep.SourceModTime = st.ModTime().UnixNano()
	ep.ReviewKind = kind
	return path
}

func TestEpisodeReviewRequiresExplicitRetry(t *testing.T) {
	setupTestDB(t)
	sub := seedSubscription(t, database.Subscription{TitleCN: "测试"})
	claim := fmt.Sprintf("%d/1/4", sub.ID)
	alt := database.Episode{SubscriptionID: sub.ID, Season: 1, Number: 4, Status: "organized", ReservationKey: &claim}
	ep := database.Episode{SubscriptionID: sub.ID, Season: 1, TorrentHash: "AABB", Status: "downloaded", ManualStatus: "pending", SourceFile: "盘一/未识别.mkv", DownloadRoot: "/downloads"}
	bindReviewedFile(t, &ep, database.ReviewEpisodeIdentity)
	for _, row := range []*database.Episode{&alt, &ep} {
		if err := database.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	reason := "所选文件的集号不明确"
	changed, err := database.MarkEpisodeForReview(t.Context(), ep, reason)
	if err != nil || !changed {
		t.Fatalf("未记录待复核：%v, %v", changed, err)
	}
	s := newTestServer()
	url := fmt.Sprintf("/api/episodes/%d/retry-organize", ep.ID)
	list := webRequest(s, "GET", fmt.Sprintf("/api/subscriptions/%d", sub.ID), "")
	var detail struct{ Episodes []map[string]any }
	if err := json.Unmarshal(list.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range detail.Episodes {
		if item["id"] == float64(ep.ID) {
			found = item["status"] == "needs_review" && item["review_reason"] == reason && item["can_assign_episode"] == true && item["source_file"] == ep.SourceFile
		}
	}
	if !found {
		t.Fatalf("详情未显示复核文件：%s", list.Body.String())
	}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	w := httptest.NewRecorder()
	auth.AuthMiddleware(mux).ServeHTTP(w, httptest.NewRequest("POST", url, nil))
	if w.Code != 401 {
		t.Fatalf("未登录重试得到 %d", w.Code)
	}

	for _, body := range []string{`{"season":2}`, `{"number":4}`, `{"season":100,"number":1}`, `{"season":0,"number":0}`, `{"season":1,"number":2001}`, `{"season":-1,"number":1}`, `{"source_file":"/etc/passwd"}`, `{"season":0,"number":1}{}`, `null`} {
		if w := webRequest(s, "POST", url, body); w.Code != 400 {
			t.Errorf("应拒绝 %s, HTTP %d", body, w.Code)
		}
	}
	w = webRequest(s, "POST", url, `{"season":1,"number":4}`)
	if w.Code != 200 {
		t.Fatalf("同一集另一个版本可指派：%d %s", w.Code, w.Body.String())
	}
	var got database.Episode
	if err := database.DB.First(&got, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "downloaded" || got.ReviewReason != "" || !got.HasConfirmedIdentity(database.ReviewEpisodeIdentity, ep.FileSize, ep.SourceModTime) || got.Season != 1 || got.Number != 4 || got.ManualStatus != "pending" || got.SourceFile != ep.SourceFile || got.ReservationKey == nil || *got.ReservationKey == claim {
		t.Fatalf("重试状态/文件归属错误：%+v", got)
	}
	if changed, err := database.MarkEpisodeForReview(t.Context(), ep, reason); changed || err != nil {
		t.Fatalf("过期调度结果覆盖人工重试：%v %v", changed, err)
	}
	if w := webRequest(s, "POST", url, ""); w.Code != 409 {
		t.Fatalf("重复重试得到 %d", w.Code)
	}
	if err := database.DB.First(&alt, alt.ID).Error; err != nil || alt.ReservationKey == nil || *alt.ReservationKey != claim || alt.Status != "organized" {
		t.Fatalf("其他版本被改写: %+v, %v", alt, err)
	}
	got.ReviewKind = database.ReviewEpisodeIdentity
	changed, err = database.MarkEpisodeForReview(t.Context(), got, reason)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	if w := webRequest(s, "POST", url, `{"season":0,"number":1.5}`); w.Code != 200 {
		t.Fatalf("特典应能赋号: %d %s", w.Code, w.Body.String())
	}
	if err := database.DB.First(&got, ep.ID).Error; err != nil || got.Season != 0 || got.Number != 1.5 {
		t.Fatalf("特典号未保存：%+v %v", got, err)
	}
}

func TestEpisodeReviewUnboundOrDeleted(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	for _, src := range []string{"", ".", "../outside.mkv", "/outside.mkv"} {
		ep := database.Episode{SubscriptionID: 1, Status: "needs_review", SourceFile: src, DownloadRoot: "/data", ReviewReason: "无法确认源"}
		if err := database.DB.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
		url := fmt.Sprintf("/api/episodes/%d/retry-organize", ep.ID)
		if w := webRequest(s, "POST", url, `{"season":1,"number":2}`); w.Code != 409 {
			t.Fatalf("未绑定源 %q 应拒绝赋号：%d", src, w.Code)
		}
		if w := webRequest(s, "POST", url, ""); w.Code != 200 {
			t.Fatalf("仅重试：%d", w.Code)
		}
		if err := database.DB.First(&ep, ep.ID).Error; err != nil || ep.Status != "downloaded" || ep.ReviewReason != "" || ep.Number != 0 {
			t.Fatalf("仅重试改动了身份：%+v %v", ep, err)
		}
		if err := database.DB.Delete(&ep).Error; err != nil {
			t.Fatal(err)
		}
		if changed, err := database.MarkEpisodeForReview(t.Context(), ep, "复核"); changed || err != nil {
			t.Fatalf("已删除行被改写：%v %v", changed, err)
		}
		if w := webRequest(s, "POST", url, ""); w.Code != 404 {
			t.Fatalf("已删除行：%d", w.Code)
		}
	}
	for _, id := range []string{"0", "bad", "18446744073709551615"} {
		if w := webRequest(s, "POST", "/api/episodes/"+id+"/retry-organize", ""); w.Code != 400 {
			t.Fatalf("无效 ID %q：%d", id, w.Code)
		}
	}
}

func TestEpisodeReviewSourceAlreadyClaimed(t *testing.T) {
	setupTestDB(t)
	newEpisode := database.Episode{SubscriptionID: 9, Status: "needs_review", ReviewReason: "集号不明", TorrentHash: "ABCD0123456789ABCDEF0123456789ABCDEF0123", SourceFile: "E04.mkv", DownloadRoot: "/downloads"}
	bindReviewedFile(t, &newEpisode, database.ReviewEpisodeIdentity)
	key := database.EpisodeFileReservationKey(9, "abcd0123456789abcdef0123456789abcdef0123", "E04.mkv")
	original := database.Episode{SubscriptionID: 9, Status: "organized", SourceFile: "E04.mkv", ReservationKey: &key, FinalPath: "Season 1/E04.mkv"}
	for _, e := range []*database.Episode{&original, &newEpisode} {
		if err := database.DB.Create(e).Error; err != nil {
			t.Fatal(err)
		}
	}
	url := fmt.Sprintf("/api/episodes/%d/retry-organize", newEpisode.ID)
	if w := webRequest(newTestServer(), "POST", url, `{"season":1,"number":4}`); w.Code != 409 {
		t.Fatalf("源已被占用：%d %s", w.Code, w.Body.String())
	}
	if err := database.DB.First(&newEpisode, newEpisode.ID).Error; err != nil || newEpisode.Status != "needs_review" || newEpisode.ReviewReason != "集号不明" || newEpisode.Number != 0 {
		t.Fatalf("冲突改变了原记录 %+v %v", newEpisode, err)
	}
}

func TestEpisodeReviewMovieIsExplicitAndSourceBound(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	ep := database.Episode{SubscriptionID: 1, Status: "needs_review", SourceFile: "movie.mkv", TorrentHash: "moviehash", Number: 29, ReviewReason: "电影包需确认正片"}
	file := bindReviewedFile(t, &ep, database.ReviewMovieSelection)
	if err := database.DB.Create(&ep).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/episodes/%d/retry-organize", ep.ID)
	dto := toEpisodeResponse(ep)
	if !dto.CanConfirmMovie || dto.CanAssignEpisode || !dto.CanRetryOrganize || dto.SourceFile != ep.SourceFile {
		t.Fatalf("电影复核能力错误 %+v", dto)
	}
	for _, body := range []string{`{"season":1,"number":29}`} {
		if w := webRequest(s, "POST", path, body); w.Code != 409 {
			t.Fatalf("不应靠空请求或季集跳过电影确认：%d", w.Code)
		}
	}
	if w := webRequest(s, "POST", path, `{"confirm_movie":false}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := webRequest(s, "POST", path, `{"confirm_movie":true,"season":1,"number":4}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := webRequest(s, "POST", path, `{"confirm_movie":true}`); w.Code != 200 {
		t.Fatalf("明确确认正片失败：%d %s", w.Code, w.Body.String())
	}
	var got database.Episode
	if err := database.DB.First(&got, ep.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "downloaded" || got.Number != 0 || !got.HasConfirmedIdentity(database.ReviewMovieSelection, ep.FileSize, ep.SourceModTime) || got.HasConfirmedIdentity(database.ReviewMovieSelection, ep.FileSize, ep.SourceModTime+1) || got.HasConfirmedIdentity(database.ReviewEpisodeIdentity, ep.FileSize, ep.SourceModTime) {
		t.Fatalf("正片确认没有与具体文件绑定：%+v", got)
	}
	got.ReviewKind = database.ReviewMovieSelection
	if ok, err := database.MarkEpisodeForReview(t.Context(), got, "源变更"); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := os.WriteFile(file, []byte("replaced the media"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := webRequest(s, "POST", path, `{"confirm_movie":true}`); w.Code != 409 {
		t.Fatalf("放行了磁盘上已变化的文件，状态码 %d", w.Code)
	}
	if err := database.DB.First(&got, ep.ID).Error; err != nil || got.Status != "needs_review" || got.ConfirmedIdentity != "" {
		t.Fatalf("复核证据被抹去：%+v %v", got, err)
	}
}

func TestTypedReviewRechecksSourceWithoutApprovingIdentity(t *testing.T) {
	setupTestDB(t)
	for _, kind := range []string{database.ReviewEpisodeIdentity, database.ReviewMovieSelection} {
		ep := database.Episode{Status: "needs_review", SubscriptionID: 1, Season: 2, Number: 29, TorrentHash: "opaque", SourceFile: "sample.mkv", DownloadRoot: t.TempDir(), ReviewReason: "源尚未核实", ReviewKind: kind}
		if err := database.DB.Create(&ep).Error; err != nil {
			t.Fatal(err)
		}
		view := toEpisodeResponse(ep)
		if !view.CanRetryOrganize || view.CanAssignEpisode || view.CanConfirmMovie {
			t.Fatalf("证据未就绪的能力错误：%+v", view)
		}
		if w := webRequest(newTestServer(), "POST", fmt.Sprintf("/api/episodes/%d/retry-organize", ep.ID), ""); w.Code != 200 {
			t.Fatalf("重验源文件：%d %s", w.Code, w.Body.String())
		}
		var got database.Episode
		if err := database.DB.First(&got, ep.ID).Error; err != nil || got.Status != "downloaded" || got.ReviewKind != kind || got.Number != 0 || got.ConfirmedIdentity != "" {
			t.Fatalf("空重试不应释放身份复核：%+v %v", got, err)
		}
	}
}

func TestEpisodeReviewLegacyLibraryFileWithoutTask(t *testing.T) {
	setupTestDB(t)
	s := newTestServer()
	for _, tc := range []struct {
		name, kind, body string
		bindFinal        bool
		want             int
	}{
		{"库内旧 E00", database.ReviewEpisodeIdentity, `{"season":1,"number":7}`, true, 200},
		{"另一库有同名文件", database.ReviewEpisodeIdentity, `{"season":2,"number":3}`, true, 200},
		{"回收站还有同一路径", database.ReviewEpisodeIdentity, `{"season":1,"number":7}`, true, 409},
		{"普通无任务下载", database.ReviewEpisodeIdentity, `{"season":1,"number":7}`, false, 409},
		{"没有任务的正片", database.ReviewMovieSelection, `{"confirm_movie":true}`, true, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ep := database.Episode{SubscriptionID: 1, Status: database.EpisodeNeedsReview, SourceFile: "剧名 S01E00.mkv"}
			source := bindReviewedFile(t, &ep, tc.kind)
			if tc.bindFinal {
				ep.FinalPath = source
			}
			if ep.ConfirmationSignature(tc.kind) != "" {
				t.Fatal("未持久化的记录不能代替任务标识")
			}
			if err := database.DB.Create(&ep).Error; err != nil {
				t.Fatal(err)
			}
			if tc.name == "回收站还有同一路径" {
				retired := database.Episode{SubscriptionID: 1, Status: "organized", FinalPath: source}
				if err := database.DB.Create(&retired).Error; err != nil {
					t.Fatal(err)
				}
				if err := database.DB.Delete(&retired).Error; err != nil {
					t.Fatal(err)
				}
			}
			w := webRequest(s, "POST", fmt.Sprintf("/api/episodes/%d/retry-organize", ep.ID), tc.body)
			if w.Code != tc.want {
				t.Fatalf("HTTP %d，想要 %d: %s", w.Code, tc.want, w.Body.String())
			}
			if tc.want != 200 {
				return
			}
			var got database.Episode
			if err := database.DB.First(&got, ep.ID).Error; err != nil || !got.HasConfirmedIdentity(tc.kind, ep.FileSize, ep.SourceModTime) {
				t.Fatalf("旧文件确认未保存：%+v %v", got, err)
			}
			got.ID++
			if got.HasConfirmedIdentity(tc.kind, ep.FileSize, ep.SourceModTime) {
				t.Fatal("另一记录沿用了确认")
			}
			got.ID--
			got.FinalPath = filepath.Join(ep.DownloadRoot, "其他.mkv")
			if got.HasConfirmedIdentity(tc.kind, ep.FileSize, ep.SourceModTime) {
				t.Fatal("不一致的历史媒体路径沿用了确认")
			}
		})
	}
}
