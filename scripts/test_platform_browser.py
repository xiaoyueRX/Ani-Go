"""用原生浏览器验收恢复后的订阅、插件、登录回跳、表单保存和库文件核对。"""

import argparse
from contextlib import closing
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import tempfile
from urllib.parse import parse_qs, quote, urlsplit

from selenium import webdriver
from selenium.common.exceptions import SessionNotCreatedException, TimeoutException
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.support.ui import WebDriverWait

from test_platform_runtime import COMMUNITY, HOST, TITLE, restored_instance, start


def open_browser(name):
    if name == "safari":
        # 首次启动时 Safari 有时还未向 Web Inspector 注册。Selenium 会关闭失败的驱动，
        # 用一份新驱动再建立一次会话。测试页面尚未打开，后续页面操作始终只执行一次。
        for attempt in range(2):
            try:
                return webdriver.Safari(service=webdriver.SafariService("/usr/bin/safaridriver"))
            except SessionNotCreatedException as exc:
                if attempt or "waiting for its RWIApplication to appear and become ready" not in (exc.msg or ""):
                    raise
                print("Safari launch timed out before a WebDriver session existed; starting a fresh driver.", flush=True)

    driver = shutil.which("msedgedriver")
    if not driver and os.environ.get("EDGEWEBDRIVER"):
        driver = shutil.which("msedgedriver", path=os.environ["EDGEWEBDRIVER"])
    if not driver:
        raise RuntimeError("EdgeDriver was not found on PATH or in EDGEWEBDRIVER")
    options = webdriver.EdgeOptions()
    options.add_argument("--headless=new")
    options.add_argument("--disable-background-networking")
    return webdriver.Edge(service=webdriver.EdgeService(driver), options=options)


def login_diagnostic(browser, base, target, log, offset):
    href, ready, session = browser.execute_script(
        "return [location.href, document.readyState, !!localStorage.getItem('token')]"
    )
    location = urlsplit(href)
    safe_path = location.path if location.path in ("/login", "/subscriptions/new", "/schedule", "/") else "/[other path]"
    safe_url = base + safe_path if location.netloc == urlsplit(base).netloc else "[different origin]"
    query = parse_qs(location.query)
    if query == {"redirect": [target]}:
        safe_url += "?redirect=" + quote(target, safe="")
    elif query == {"source": ["ci"]}:
        safe_url += "?source=ci"
    elif location.query:
        safe_url += "?[redacted]"
    if location.fragment:
        safe_url += "#form" if location.fragment == "form" else "#[redacted]"
    # 日志只分类：其中可能有用户名。只取浏览器提交之后新增的记录。
    with log.open("rb") as stream:
        stream.seek(offset)
        lines = stream.read().decode("utf-8", errors="replace").splitlines()
    outcome = "no login result"
    for line in lines:
        if "登录成功" in line:
            outcome = "accepted"
        elif "登录失败" in line:
            outcome = "unknown user" if "不存在" in line else "credential rejected"
        elif "JWT 签发失败" in line:
            outcome = "token signing failed"
    alert = " ".join(e.text for e in browser.find_elements(By.CSS_SELECTOR, '[role="alert"]') if e.is_displayed())
    public_errors = {"请输入用户名和密码", "登录失败", "用户名或密码错误", "Please enter username and password", "Login failed",
                     "Token 生成失败", "账户数据暂时不可用"}
    error = alert if alert in public_errors else ("other error" if alert else "none")
    title = browser.title if browser.title in {"Ani-Go", "Ani-Go - 全自动番剧追番管理系统"} else "[other page title]"
    return (f"final URL={safe_url}, title={title}, document={ready}, session present={session}, "
            f"WebDriver URL matches DOM={browser.current_url == href}, login API={outcome}, visible error={error}")


def fill_login_field(browser, field, value):
    field.clear()
    field.send_keys(value)
    if browser.execute_script("return arguments[0].value === arguments[1]", field, value):
        return "keyboard"
    if browser.name.lower() != "safari":
        return "keyboard mismatch"
    # SafariDriver 偶尔会改变输入字符。只对受影响的字段走原生 DOM 赋值，并通知 Vue。
    browser.execute_script("""
        const [field, value] = arguments;
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
        setter.call(field, value);
        field.dispatchEvent(new Event('input', {bubbles: true}));
        field.dispatchEvent(new Event('change', {bubbles: true}));
    """, field, value)
    return "DOM input after SafariDriver mismatch"


def check_form(browser, service, password, log):
    base = f"http://{HOST}:{service.port}"
    target = "/subscriptions/new?source=ci#form"
    title = "Browser smoke Archive"
    wait = WebDriverWait(browser, 20)
    browser.get(base + target)
    username = wait.until(EC.element_to_be_clickable((By.CSS_SELECTOR, 'form input[autocomplete="username"]')))
    login = urlsplit(browser.current_url)
    if login.path != "/login" or parse_qs(login.query).get("redirect") != [target]:
        raise RuntimeError("The login redirect did not preserve the protected URL")
    secret = browser.find_element(By.CSS_SELECTOR, 'form input[autocomplete="current-password"]')
    username_method = fill_login_field(browser, username, "admin")
    password_method = fill_login_field(browser, secret, password)
    print(f"Browser login input: username={username_method}, password={password_method}.", flush=True)
    offset = log.stat().st_size
    matches = browser.execute_script("return [arguments[0].value === 'admin', arguments[1].value === arguments[2]]",
                                     username, secret, password)
    if not all(matches):
        raise RuntimeError(f"Browser login input mismatch (username matches={matches[0]}, password matches={matches[1]}); "
                           + login_diagnostic(browser, base, target, log, offset))
    wait.until(EC.element_to_be_clickable((By.CSS_SELECTOR, 'form button[type="submit"]'))).click()
    try:
        wait.until(lambda b: b.execute_script("return window.location.href") == base + target
                   or any(e.is_displayed() for e in b.find_elements(By.CSS_SELECTOR, '[role="alert"]')))
    except TimeoutException:
        raise RuntimeError("Login did not return to the subscription form; " + login_diagnostic(browser, base, target, log, offset)) from None
    if browser.execute_script("return window.location.href") != base + target:
        raise RuntimeError("Browser login failed; " + login_diagnostic(browser, base, target, log, offset))
    title_input = (By.CSS_SELECTOR, 'form input[type="text"][required]')
    wait.until(EC.visibility_of_element_located(title_input))
    browser.refresh()
    field = wait.until(EC.visibility_of_element_located(title_input), "The form did not survive a reload")
    field.send_keys(title, Keys.ENTER)
    heading = (By.XPATH, f"//h1[normalize-space()='{title}']")
    wait.until(EC.visibility_of_element_located(heading), "The saved subscription title is missing from its detail page")

    rows = [r for r in service.api("subscriptions") if r.get("title_cn") == title]
    if len(rows) != 1:
        raise RuntimeError("The subscription created through the browser was not saved exactly once")
    if browser.current_url != base + f"/subscriptions/{rows[0]['id']}":
        raise RuntimeError("The rendered detail page and the saved subscription ID disagree")
    return rows[0]


def check_subscription_grid(browser, service, saved):
    base = f"http://{HOST}:{service.port}"
    title = saved["title_cn"]
    if saved.get("cover_url"):
        raise RuntimeError("The saved subscription must have no cover for this check")
    wait = WebDriverWait(browser, 20)
    search = (By.CSS_SELECTOR, 'input[placeholder="搜索追番标题、字幕组或年份..."]')

    def card_titles(driver):
        # 在同一次浏览器调用中读取可见标题，避免列表更新使逐个远程读取的元素失效。
        titles = driver.execute_script("""
            return Array.from(document.querySelectorAll('h3[title]'))
                .filter(h => h.checkVisibility({opacityProperty: true, visibilityProperty: true}))
                .map(h => h.innerText)
        """)
        return [" ".join(title.split()) for title in titles]

    def filter_by_title():
        field = wait.until(EC.element_to_be_clickable(search))
        field.clear()
        field.send_keys(title)
        wait.until(lambda b: card_titles(b) == [title], "Title search did not isolate the saved subscription")
        return field

    browser.get(base + "/subscriptions")
    wait.until(lambda b: {title, TITLE}.issubset(card_titles(b)), "Both subscription cards must be visible")
    heading = (By.XPATH, f"//h3[normalize-space(.)='{title}']")
    card = browser.find_element(*heading).find_element(
        By.XPATH, "ancestor::div[contains(concat(' ',normalize-space(@class),' '),' group ')][1]")
    icon = card.find_element(By.CSS_SELECTOR, "svg.lucide-image")
    if not icon.is_displayed() or card.find_elements(By.CSS_SELECTOR, "img, .skeleton"):
        raise RuntimeError("A subscription without a cover should show a static image icon without a loading skeleton")

    filter_by_title()
    title_link = wait.until(EC.element_to_be_clickable(
        (By.XPATH, f"//h3/a[@href='/subscriptions/{saved['id']}' and normalize-space(.)='{title}']")),
        "The filtered title must be a keyboard-accessible link to the saved subscription")
    title_link.send_keys(Keys.ENTER)
    wait.until(EC.visibility_of_element_located((By.XPATH, f"//h1[normalize-space(.)='{title}']")))
    if urlsplit(browser.current_url).path != f"/subscriptions/{saved['id']}":
        raise RuntimeError("Opening the filtered card did not navigate to its subscription")
    browser.back()
    field = wait.until(EC.element_to_be_clickable(search))
    wait.until(lambda b: field.get_attribute("value") == title and card_titles(b) == [title],
               "Returning to this grid entry must preserve its filter and selected card")
    field.send_keys(Keys.BACKSPACE * len(title))
    wait.until(lambda b: field.get_attribute("value") == "" and {title, TITLE}.issubset(card_titles(b)),
               "Clearing title search did not restore both cards")
    print("Verified coverless card, title filter, detail navigation and clearing the filter.", flush=True)
    print("Verified keyboard activation of the title link and return to the filtered list.", flush=True)


def check_filtered_batch_selection(browser, service, saved):
    wait = WebDriverWait(browser, 20)
    saved_title = saved["title_cn"]

    def button(label):
        return wait.until(EC.element_to_be_clickable((By.XPATH, f"//button[normalize-space(.)='{label}']")),
                          f"Filtered batch selection did not offer {label}")

    def click_in_view(element):
        # WebDriver 从上方滚回控件时可能将其贴在固定顶栏后面。
        browser.execute_script("arguments[0].scrollIntoView({block: 'center'})", element)
        element.click()

    def filter_to(title):
        field = browser.find_element(By.CSS_SELECTOR, 'input[placeholder="搜索追番标题、字幕组或年份..."]')
        field.clear()
        field.send_keys(title)
        only_card = (By.XPATH, f"//h3[@title and normalize-space(.)='{title}' and count(//h3[@title])=1]")
        wait.until(EC.visibility_of_element_located(only_card), f"The filter should show only {title}")

    def selected_count(n):
        wait.until(EC.visibility_of_element_located((By.XPATH, f"//span[normalize-space(.)='已选 {n}']")))

    click_in_view(button("批量删除"))
    filter_to(saved_title)
    row = wait.until(EC.element_to_be_clickable((By.XPATH, f"//h3[normalize-space(.)='{saved_title}']")))
    click_in_view(row)
    selected_count(1)
    filter_to(TITLE)
    click_in_view(button("全选本页"))
    selected_count(2)
    filter_to(saved_title)
    click_in_view(button("取消全选"))
    selected_count(1)
    filter_to(TITLE)
    button("取消全选")  # 取消另一组后，这张卡仍已选中。
    click_in_view(button("取消"))
    button("批量删除")
    if {s["title_cn"] for s in service.api("subscriptions")} != {saved_title, TITLE}:
        raise RuntimeError("Canceling batch selection should leave both subscriptions intact")
    print("Verified filtered batch selection keeps hidden selections and cancels without deletion.", flush=True)


def check_restored_plugin(browser, service):
    base = f"http://{HOST}:{service.port}"
    wait = WebDriverWait(browser, 20)
    browser.get(base + "/subscriptions")
    title = wait.until(EC.element_to_be_clickable((By.XPATH, f"//h3[normalize-space()='{TITLE}']")))
    browser.execute_script("arguments[0].scrollIntoView({block: 'center'})", title)
    title.click()
    wait.until(EC.visibility_of_element_located((By.XPATH, f"//h1[normalize-space()='{TITLE}']")))

    browser.get(base + "/settings?tab=plugins")
    toggle = (By.XPATH, "//h4[normalize-space()='Cloud Tasks']"
                        "/ancestor::div[.//input[@type='checkbox']][1]//input[@type='checkbox']")
    control = wait.until(EC.element_to_be_clickable(toggle))
    if control.is_selected():
        raise RuntimeError("Restored community plugin unexpectedly appears enabled in settings")
    browser.execute_script("arguments[0].scrollIntoView({block: 'center'})", control)
    control.click()
    wait.until(EC.element_located_selection_state_to_be(toggle, True), "Plugin did not enable after click")
    browser.refresh()
    wait.until(EC.element_located_selection_state_to_be(toggle, True), "Plugin enablement did not survive reloading settings")
    custom = service.api("plugins/custom")
    if not any(p["id"] == COMMUNITY and p["enabled"] and p["status"] == "active" for p in custom):
        raise RuntimeError("The API did not confirm the restored plugin was activated through the browser")


def library_history(dbpath, sub_id):
    with closing(sqlite3.connect(dbpath.as_uri() + "?mode=ro", uri=True)) as db:
        progress = db.execute("""SELECT current_episodes, total_episodes, completed, enabled, updated_at
                                 FROM subscriptions WHERE id = ?""", (sub_id,)).fetchone()
        episodes = db.execute("SELECT * FROM episodes WHERE subscription_id = ? ORDER BY id", (sub_id,)).fetchall()
        return progress, episodes


def check_library_files(browser, service, home):
    # 在同一个已停用的合成订阅中生成两条入库历史，再移走 E02，只留下历史记录。
    sub, = [row for row in service.api("subscriptions") if row["title_cn"] == TITLE]
    route = f"subscriptions/{sub['id']}"
    folder = home / "剧集" / TITLE / "Season 2"
    folder.mkdir(parents=True)
    present, missing = folder / "S02E01.mkv", folder / "S02E02.mkv"
    contents = b"Ani-Go synthetic episode 1\n" * 64
    dbpath = home / "订阅 数据.sqlite"
    with closing(sqlite3.connect(dbpath)) as db, db:
        for number, path, data in [(1, present, contents), (2, missing, b"Ani-Go synthetic episode 2\n" * 128)]:
            path.write_bytes(data)
            db.execute("""INSERT INTO episodes
                (subscription_id, season, number, status, original_name, source_file, source_mod_time,
                 final_path, file_size, organized_at, created_at, updated_at)
                VALUES (?, 2, ?, 'organized', ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)""",
                       (sub["id"], number, path.name, path.name, path.stat().st_mtime_ns, str(path), len(data)))
    service.api(route, "PUT", {"total_episodes": 2})
    missing.unlink()
    before = library_history(dbpath, sub["id"])
    if before[0][:4] != (2, 2, True, False) or len(before[1]) != 2:
        raise RuntimeError("Expected two completed episodes before the library check")

    browser.get(f"http://{HOST}:{service.port}/{route}")
    wait = WebDriverWait(browser, 20)
    progress = (By.XPATH, "//p[translate(normalize-space(.), ' ', '')='2/2']")
    organized = (By.XPATH, "//tbody/tr/td/span[normalize-space(.)='已入库']")

    def original_progress_visible(driver):
        return (EC.visibility_of_element_located(progress)(driver)
                and len([e for e in driver.find_elements(*organized) if e.is_displayed()]) == 2)

    wait.until(original_progress_visible, "The detail page did not show 2/2 with two organized episodes")
    button = wait.until(EC.element_to_be_clickable((By.XPATH, "//button[normalize-space(.)='核对全部已整理文件']")),
                        "Missing the manual library file check on the subscription detail page")
    browser.execute_script("arguments[0].scrollIntoView({block: 'center'})", button)
    button.click()

    report = wait.until(EC.visibility_of_element_located((By.XPATH, "//details[summary[contains(., '库文件核对')]]")))
    # SafariDriver 会为列表项添加尾随空格，按钮也可能由换行分隔。逐项比对完整文字。
    rows = sorted(" ".join(item.text.split()) for item in report.find_elements(By.TAG_NAME, "li"))
    expected = sorted([f"{present.name} · 文件存在，大小与记录一致", f"{missing.name} · 当前路径未找到文件 从下载原件补回"])
    if rows != expected:
        raise RuntimeError(f"Library file check returned unexpected rows or repair actions: {rows!r}")
    if not original_progress_visible(browser):
        raise RuntimeError("The file check changed the rendered progress or organized status")
    if library_history(dbpath, sub["id"]) != before or present.read_bytes() != contents or missing.exists():
        raise RuntimeError("The file check changed the stored progress, episode history or media files")


def check_mobile_header(browser, service):
    # Safari 有最小窗口宽度；同源框架可让两个原生浏览器使用相同的 360 CSS 像素布局。
    frame = browser.execute_script("""
        const frame = document.createElement('iframe');
        frame.style.cssText = 'position:fixed;inset:0 auto auto 0;width:360px;height:640px;border:0;z-index:2147483647';
        frame.src = '/subscriptions';
        return document.body.appendChild(frame);
    """)
    try:
        browser.switch_to.frame(frame)
        wait = WebDriverWait(browser, 20)
        wait.until(EC.visibility_of_element_located((By.CSS_SELECTOR, "main h3[title]")))
        version = service.api("health")["version"]
        footer = wait.until(EC.visibility_of_element_located((By.CSS_SELECTOR, "footer span.font-mono")))
        if not version or footer.text != version:
            raise RuntimeError("The mobile footer must show the full running version")
        menu = browser.find_element(By.CSS_SELECTOR, "header button")
        account = browser.find_element(By.CSS_SELECTOR, "header .avatar")
        dimensions = browser.execute_script("""
            const [menu, account, version] = arguments, root = document.documentElement;
            const text = document.createRange();
            text.selectNodeContents(version);
            const rects = [menu.getBoundingClientRect(), account.getBoundingClientRect(), ...text.getClientRects()];
            return [innerWidth, root.clientWidth, root.scrollWidth,
                    rects.every(r => r.width > 0 && r.left >= -1 && r.right <= root.clientWidth + 1)];
        """, menu, account, footer)
        viewport, layout, content, fits = dimensions
        if viewport != 360 or content > layout + 1 or not fits:
            raise RuntimeError(f"Mobile header or version extends outside the 360px page: "
                               f"viewport={viewport}, layout={layout}, content={content}, controls and version fit={fits}")

        account.click()
        wait.until(EC.visibility_of_element_located((By.XPATH, "//header//li[normalize-space(.)='admin']")))
        menu.click()
        settings = (By.CSS_SELECTOR, "body > div.fixed a[href='/settings']")
        wait.until(EC.element_to_be_clickable(settings)).click()
        wait.until(lambda b: b.execute_script("return location.pathname") == "/settings")
        print(f"Verified 360px mobile menus and full version {version}; layout/content={layout}/{content}.", flush=True)
    finally:
        browser.switch_to.default_content()
        browser.execute_script("arguments[0].remove()", frame)


def check_filter_layout(browser, command, root, offline):
    # 用同一浏览器和独立空库，保留先前登录、备份恢复和媒体核对的数据。
    with socket.socket() as available:
        available.bind((HOST, 0))
        port = available.getsockname()[1]
    with start(command, root / "filter", root / "filter-backups", port, secrets.token_urlsafe(24), offline) as service:
        titles = ["Grid XZ"] + [f"Grid X {i}" for i in range(14)] + [f"Grid Y {i}" for i in range(5)]
        reply = service.api("subscriptions/batch", "POST", {"items": [{"title_cn": title} for title in titles]}, status=201)
        if reply["failed"] or len(reply["success"]) != len(titles):
            raise RuntimeError("Could not create the isolated filter sample")
        base = f"http://{HOST}:{port}"
        browser.get(base + "/login")
        browser.execute_script("localStorage.setItem('token', arguments[0]);"
                               "localStorage.setItem('ani-go-setup-wizard-dismissed', 'true')", service.token)
        browser.get(base + "/subscriptions")
        wait = WebDriverWait(browser, 20)
        field = wait.until(EC.element_to_be_clickable((By.CSS_SELECTOR, 'input[placeholder="搜索追番标题、字幕组或年份..."]')))

        def expect_titles(expected):
            wait.until(lambda b: set(b.execute_script(
                "return Array.from(document.querySelectorAll('h3[title]'), h => h.title)")) == set(expected))

        samples = []
        for key in ("X", "Z"):
            expect_titles(titles)
            matches = [title for title in titles if key in title]
            # 旧网格每移除一张卡就读 body.offsetHeight。透传原生 getter，仅在这次筛选期间计数。
            browser.execute_script("""
                const getter = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetHeight').get;
                let reads = 0;
                Object.defineProperty(document.body, 'offsetHeight', {
                    configurable: true, get() { reads++; return getter.call(this); }
                });
                window.finishFilterProbe = () => {
                    delete document.body.offsetHeight;
                    delete window.finishFilterProbe;
                    return reads;
                };
            """)
            try:
                field.send_keys(key)
                expect_titles(matches)
            finally:
                reads = browser.execute_script("return window.finishFilterProbe()")
            samples.append((len(titles) - len(matches), reads))
            field.send_keys(Keys.BACKSPACE)
        expect_titles(titles)
        (few, few_reads), (many, many_reads) = samples
        print(f"Filter removed {few}/{many} cards; document body height reads: {few_reads}/{many_reads}.", flush=True)
        # 允许固定的浏览器开销；多移除的卡片不应每两张就引入一次额外的整页几何读取。
        if 2 * (many_reads - few_reads) >= many - few:
            raise RuntimeError("Document layout reads scaled with the number of filtered-out cards")
        check_mobile_header(browser, service)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--browser", choices=("edge", "safari"), required=True)
    args = parser.parse_args()
    command = [str(args.binary.resolve(strict=True))]
    with tempfile.TemporaryDirectory(prefix="Ani-Go browser ") as directory, socket.socket() as blocked:
        root = Path(directory)
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        with socket.socket() as free:
            free.bind((HOST, 0))
            port = free.getsockname()[1]
        with restored_instance(command, root, port, offline) as (service, password, home):
            with open_browser(args.browser) as browser:
                browser.set_page_load_timeout(30)
                saved = check_form(browser, service, password, home / "data" / "ani-go.log")
                check_subscription_grid(browser, service, saved)
                check_filtered_batch_selection(browser, service, saved)
                check_restored_plugin(browser, service)
                check_library_files(browser, service, home)
                check_filter_layout(browser, command, root, offline)
                print(f"Passed: {browser.name} {browser.capabilities.get('browserVersion')}, login, "
                      "subscription save, restored plugin activation, present and missing library files, "
                      "unchanged 2/2 progress and organized history.")


if __name__ == "__main__":
    main()
