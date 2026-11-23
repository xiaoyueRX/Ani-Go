"""用 runner 自带的 Edge 或 Safari 验收内嵌网页的登录回跳、刷新和保存。"""

import argparse
import os
from pathlib import Path
import secrets
import shutil
import socket
import tempfile
from urllib.parse import parse_qs, urlsplit

from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.support.ui import WebDriverWait

from test_platform_runtime import HOST, start


def open_browser(name):
    if name == "safari":
        return webdriver.Safari(service=webdriver.SafariService("/usr/bin/safaridriver"))

    driver = shutil.which("msedgedriver")
    if not driver and os.environ.get("EDGEWEBDRIVER"):
        driver = shutil.which("msedgedriver", path=os.environ["EDGEWEBDRIVER"])
    if not driver:
        raise RuntimeError("EdgeDriver was not found on PATH or in EDGEWEBDRIVER")
    options = webdriver.EdgeOptions()
    options.add_argument("--headless=new")
    options.add_argument("--disable-background-networking")
    return webdriver.Edge(service=webdriver.EdgeService(driver), options=options)


def check_form(browser, service, password):
    base = f"http://{HOST}:{service.port}"
    target = "/subscriptions/new?source=ci#form"
    title = "Browser smoke Archive"
    wait = WebDriverWait(browser, 20)
    browser.get(base + target)
    wait.until(EC.visibility_of_element_located((By.CSS_SELECTOR, 'form input[autocomplete="username"]'))).send_keys("admin")
    login = urlsplit(browser.current_url)
    if login.path != "/login" or parse_qs(login.query).get("redirect") != [target]:
        raise RuntimeError("The login redirect did not preserve the protected URL")
    browser.find_element(By.CSS_SELECTOR, 'form input[autocomplete="current-password"]').send_keys(password)
    browser.find_element(By.CSS_SELECTOR, 'form button[type="submit"]').click()
    wait.until(EC.url_to_be(base + target), "Login did not return to the subscription form")
    title_input = (By.CSS_SELECTOR, 'form input[type="text"][required]')
    wait.until(EC.visibility_of_element_located(title_input))
    browser.refresh()
    field = wait.until(EC.visibility_of_element_located(title_input), "The form did not survive a reload")
    field.send_keys(title, Keys.ENTER)
    heading = (By.XPATH, f"//h1[normalize-space()='{title}']")
    wait.until(EC.visibility_of_element_located(heading), "The saved subscription title is missing from its detail page")

    rows = service.api("subscriptions")
    if len(rows) != 1 or rows[0].get("title_cn") != title:
        raise RuntimeError("The subscription created through the browser was not saved exactly once")
    if browser.current_url != base + f"/subscriptions/{rows[0]['id']}":
        raise RuntimeError("The rendered detail page and the saved subscription ID disagree")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--browser", choices=("edge", "safari"), required=True)
    args = parser.parse_args()
    command = [str(args.binary.resolve(strict=True))]
    password = secrets.token_urlsafe(32)
    with tempfile.TemporaryDirectory(prefix="Ani-Go browser ") as directory, socket.socket() as blocked:
        root = Path(directory)
        blocked.bind((HOST, 0))
        offline = f"http://{HOST}:{blocked.getsockname()[1]}"
        with socket.socket() as free:
            free.bind((HOST, 0))
            port = free.getsockname()[1]
        with start(command, root, root / "backups", port, password, offline) as service:
            with open_browser(args.browser) as browser:
                browser.set_page_load_timeout(30)
                check_form(browser, service, password)
                print(f"Passed: {browser.name} {browser.capabilities.get('browserVersion')}, login redirect, reload and subscription save.")


if __name__ == "__main__":
    main()
