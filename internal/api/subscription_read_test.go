package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/xiaoyueRX/Ani-Go/internal/database"
	"gorm.io/gorm"
)

func TestSubscriptionDetailReportsDatabaseReadFailures(t *testing.T) {
	for _, table := range []string{"subscriptions", "episodes"} {
		t.Run(table, func(t *testing.T) {
			setupTestDB(t)
			if err := database.DB.Create(&database.Subscription{TitleCN: "示例"}).Error; err != nil {
				t.Fatal(err)
			}
			callback := database.DB.Callback().Query()
			err := callback.Before("gorm:query").Register("test:detail-read", func(tx *gorm.DB) {
				if tx.Statement.Table == table && len(tx.Statement.Selects) == 0 {
					tx.AddError(errors.New("fixture read failure"))
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			defer callback.Remove("test:detail-read")
			mux := http.NewServeMux()
			(&Server{}).registerRoutes(mux)
			panelCall(t, mux, "GET", "/api/subscriptions/1", "", http.StatusInternalServerError)
		})
	}
}
