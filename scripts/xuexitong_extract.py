#!/usr/bin/env python3
"""
学习通作业题目摘取器 — 从历史作业中提取题目用于复习
基于 yatori-go-core 的 API 逆向分析
"""
import requests
import re
import json
import time
import sys
import os
from urllib.parse import urlparse, parse_qs, urlencode
from Crypto.Cipher import AES
from Crypto.Util.Padding import pad
from base64 import b64encode
from bs4 import BeautifulSoup
from dataclasses import dataclass, field, asdict
from typing import Optional

# ============ 配置 ============
PHONE = "15729572880"
PASSWORD = "yuefei1997"
OUTPUT_DIR = "/root/x/Ani-Go/xuexitong_questions"

# AES 加密配置（与 yatori-go-core 一致）
AES_KEY = b"u2oh6Vu^HWe4_AES"  # 16 bytes
APP_VERSION = "6.7.2"
BUILD = "10941_314"
DEVICE_VENDOR = "MI10"
ANDROID_VERSION = "Android 16"
IMEI = "a1b2c3d4e5f67890"  # 固定一个，不影响功能

# 要提取的课程名（空列表 = 全部课程）
TARGET_COURSES = []  # 如 ["高等数学", "大学物理"]

# ============ 数据结构 ============
@dataclass
class Question:
    index: int
    qtype: str          # 单选题/多选题/判断题/填空题/简答题/论述题
    qtype_code: str     # 0-6
    question_id: str
    content: str
    options: list = field(default_factory=list)
    
@dataclass
class WorkItem:
    name: str
    status: str
    course_name: str
    questions: list = field(default_factory=list)

# ============ 工具函数 ============
def aes_encrypt(plaintext: str) -> str:
    """AES-128-CBC 加密，key 和 IV 相同"""
    cipher = AES.new(AES_KEY, AES.MODE_CBC, AES_KEY)
    padded = pad(plaintext.encode(), AES.block_size)
    encrypted = cipher.encrypt(padded)
    return b64encode(encrypted).decode()

def get_ua():
    return (
        f"Mozilla/5.0 (Linux; {ANDROID_VERSION}; {DEVICE_VENDOR} Build/OPM1.171019.019; wv) "
        f"AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/71.0.3578.99 Mobile Safari/537.36 "
        f"(schild:somehash) (device:{DEVICE_VENDOR}) Language/zh_CN "
        f"com.chaoxing.mobile/ChaoXingStudy_3_{APP_VERSION}_android_phone_{BUILD} "
        f"(@Kalimdor)_{IMEI}"
    )

def extract_text(soup, selector):
    """安全提取文本"""
    el = soup.select_one(selector)
    return el.get_text(strip=True) if el else ""

def clean_html_text(text: str) -> str:
    """清理 HTML 实体和多余空白"""
    text = text.replace("\xa0", " ").replace("\u3000", " ")
    text = re.sub(r'\s+', ' ', text).strip()
    return text

# ============ 登录 ============
def login() -> requests.Session:
    """登录学习通，返回带 cookies 的 session"""
    session = requests.Session()
    session.verify = False
    session.headers.update({"User-Agent": get_ua()})
    
    encrypted_phone = aes_encrypt(PHONE)
    encrypted_pass = aes_encrypt(PASSWORD)
    
    resp = session.post(
        "https://passport2.chaoxing.com/fanyalogin",
        data={
            "fid": "-1",
            "uname": encrypted_phone,
            "password": encrypted_pass,
            "refer": "http%3A%2F%2Fi.mooc.chaoxing.com",
            "t": "true",
            "forbidotherlogin": "0",
            "validate": "",
            "doubleFactorLogin": "0",
            "independentId": "0",
            "independentNameId": "0",
        },
        allow_redirects=True
    )
    
    if "很抱歉" in resp.text:
        raise Exception(f"登录被风控拦截: {resp.text[:200]}")
    
    print(f"✅ 登录完成，cookies: {len(session.cookies)} 个")
    return session

# ============ 课程列表 ============
def get_courses(session: requests.Session) -> list[dict]:
    """拉取课程列表"""
    resp = session.get(
        "https://mooc1-api.chaoxing.com/mycourse/backclazzdata",
        headers={"User-Agent": get_ua()}
    )
    data = resp.json()
    
    courses = []
    for ch in data.get("channelList", []):
        # 跳过没有 course data 的项（如分类夹）
        course_data = ch.get("content", {}).get("course", {}).get("data", [])
        if not course_data:
            continue
        
        # classId 在 channel 的 key 字段，cpi 在 channel 或 content 层
        class_id = str(ch.get("key", ""))
        cpi = str(ch.get("cpi", "") or ch.get("content", {}).get("cpi", ""))
        
        for c in course_data:
            if c.get("name"):
                courses.append({
                    "name": c["name"],
                    "courseId": str(c.get("id", "")),
                    "classId": class_id,
                    "cpi": cpi,
                })
    
    print(f"📚 拉取到 {len(courses)} 门课程")
    for i, c in enumerate(courses):
        print(f"   [{i+1}] {c['name']} (courseId={c['courseId']}, classId={c['classId']})")
    
    return courses

# ============ 作业列表 ============
def get_work_list(session: requests.Session, course: dict) -> list[dict]:
    """拉取课程作业列表（包括已完成的）"""
    url = (
        f"https://mooc1-api.chaoxing.com/work/task-list"
        f"?courseId={course['courseId']}&classId={course['classId']}&cpi={course['cpi']}"
    )
    resp = session.get(url, headers={"User-Agent": get_ua()})
    soup = BeautifulSoup(resp.text, "html.parser")
    
    works = []
    for li in soup.select("ul.nav li"):
        raw_url = li.get("data", "")
        if not raw_url:
            continue
        
        params = parse_qs(urlparse(raw_url).query)
        name_el = li.select_one("div p")
        status_els = li.select("div span")
        
        name = name_el.get_text(strip=True) if name_el else "未知作业"
        status = status_els[0].get_text(strip=True) if status_els else "未知"
        
        works.append({
            "name": name,
            "status": status,
            "taskrefId": params.get("taskrefId", [""])[0],
            "courseId": params.get("courseId", [course['courseId']])[0],
            "userId": params.get("userId", [""])[0],
            "clazzId": params.get("clazzId", [course['classId']])[0],
            "type": params.get("type", [""])[0],
            "enc_task": params.get("enc_task", [""])[0],
            "msgId": params.get("msgId", ["0"])[0],
        })
    
    return works

# ============ 进入作业 ============
def enter_work(session: requests.Session, work: dict) -> Optional[dict]:
    """进入作业，获取 cpi/answerId/enc 等关键参数"""
    url = (
        f"https://mooc1-api.chaoxing.com/android/mtaskmsgspecial"
        f"?taskrefId={work['taskrefId']}&msgId={work['msgId']}"
        f"&courseId={work['courseId']}&userId={work['userId']}"
        f"&clazzId={work['clazzId']}&type={work['type']}&enc_task={work['enc_task']}"
    )
    resp = session.get(url, headers={"User-Agent": get_ua()}, allow_redirects=True)
    html = resp.text
    
    if "已过时效" in html:
        return None  # 过期作业，可能无法进入
    
    # 提取题目数量
    match = re.search(r'共包含\s*(\d+)\s*道题目', html)
    if not match:
        match = re.search(r'共\s*(\d+)\s*题', html)
    question_total = int(match.group(1)) if match else 0
    
    # 提取 cpi, workAnswerId, enc
    cpi = ""
    answer_id = ""
    enc = ""
    
    cpi_match = re.search(r'cpi=(\d+)', html)
    if cpi_match:
        cpi = cpi_match.group(1)
    
    answer_match = re.search(r'workAnswerId=(\d+)', html)
    if answer_match:
        answer_id = answer_match.group(1)
    
    enc_match = re.search(r'enc=([a-fA-F0-9]+)', html)
    if enc_match:
        enc = enc_match.group(1)
    
    if not all([cpi, answer_id, enc]):
        # 从隐藏表单中提取
        soup = BeautifulSoup(html, "html.parser")
        for inp in soup.select("input[type='hidden']"):
            id_val = inp.get("id", "")
            if id_val == "cpi":
                cpi = inp.get("value", "")
            elif id_val == "testUserRelationId":
                answer_id = inp.get("value", "")
    
    if not enc:
        # 尝试从跳转 URL 中提取
        final_url = resp.url
        enc_match2 = re.search(r'enc=([a-fA-F0-9]+)', final_url)
        if enc_match2:
            enc = enc_match2.group(1)
    
    return {
        "cpi": cpi,
        "answerId": answer_id,
        "enc": enc,
        "questionTotal": question_total,
    }

# ============ 获取作业试卷（获取 enc 等参数）============
def get_work_paper(session: requests.Session, work: dict, enter_info: dict) -> Optional[dict]:
    """获取作业试卷页面，提取加密参数"""
    url = (
        f"https://mooc1-api.chaoxing.com/mooc-ans/work/phone/doHomeWork"
        f"?courseId={work['courseId']}&workId={work['taskrefId']}"
        f"&cpi={enter_info['cpi']}&workAnswerId={enter_info['answerId']}"
        f"&classId={work['clazzId']}&oldWorkId&mooc=1&msgId={work['msgId']}"
        f"&source=0&checkIntegrity=true&enc={enter_info['enc']}"
        f"&keyboardDisplayRequiresUserAction=1"
    )
    resp = session.get(url, headers={"User-Agent": get_ua()})
    soup = BeautifulSoup(resp.text, "html.parser")
    
    result = {
        "enc": "",
        "encRemainTime": "",
        "encLastUpdateTime": "",
        "encWork": "",
        "questionTotal": enter_info["questionTotal"],
    }
    
    for field in ["enc", "encRemainTime", "encLastUpdateTime", "encWork"]:
        el = soup.select_one(f"#{field}")
        if el:
            result[field] = el.get("value", "")
    
    return result

# ============ 逐题拉取 & 解析 ============
def parse_question_html(html: str, index: int) -> Optional[Question]:
    """解析单道题目的 HTML"""
    soup = BeautifulSoup(html, "html.parser")
    
    # 题目 ID
    qid_el = soup.select_one("#questionId")
    if not qid_el:
        return None
    qid = qid_el.get("value", "")
    
    # 题型代码
    type_el = soup.select_one(f'input[name="type{qid}"]')
    type_code = type_el.get("value", "") if type_el else ""
    
    # 题型文本
    type_span = soup.select_one("span.focusSpan")
    type_text = type_span.get("aria-label", "") if type_span else ""
    type_text = re.sub(r'^\s*\d+\.\s*', '', type_text)
    
    # 真题干
    title_sel = soup.select_one("div.ans-cc.timuStyle.fontLabel.workWrap")
    if not title_sel:
        title_sel = soup.select_one("div.ans-cc.workWrap")
    if not title_sel:
        title_sel = soup.select_one(".workWrap")
    
    content = clean_html_text(title_sel.get_text()) if title_sel else ""
    
    question = Question(
        index=index,
        qtype=type_text,
        qtype_code=type_code,
        question_id=qid,
        content=content,
    )
    
    # 解析选项（按题型）
    single_div = soup.select_one("div.singleQuesId")
    if not single_div:
        return question
    
    if type_code in ("0", "1"):  # 单选/多选
        for opt in single_div.select("div.centerSpan"):
            letter = opt.get("id", "")
            text = clean_html_text(opt.get_text())
            if letter and text:
                question.options.append(f"{letter}. {text}")
    
    elif type_code == "2":  # 填空
        for blank in single_div.select(".blankItemName"):
            label = blank.get("aria-label", "")
            if label:
                question.options.append(f"空: {label}")
    
    elif type_code == "3":  # 判断
        for ans in single_div.select(".Answer"):
            letter = ans.select_one("span.check")
            text_el = ans.select_one("p.fl")
            l_text = letter.get_text(strip=True) if letter else ""
            t_text = text_el.get_text(strip=True) if text_el else ""
            if l_text or t_text:
                question.options.append(f"{l_text} {t_text}".strip())
    
    # type_code 4 (简答) 和 6 (论述) 没有选项
    
    return question

def get_question(session: requests.Session, work: dict, paper: dict, index: int) -> Optional[Question]:
    """拉取第 index 道题"""
    url = (
        f"https://mooc1-api.chaoxing.com/mooc-ans/work/phone/doHomeWork"
        f"?courseId={work['courseId']}&workId={work['taskrefId']}"
        f"&cpi={paper.get('cpi', '')}&workAnswerId={paper.get('answerId', '')}"
        f"&classId={work['clazzId']}&mooc=1"
        f"&source=0&enc={paper['enc']}&keyboardDisplayRequiresUserAction=1"
        f"&index={index}"
    )
    resp = session.get(url, headers={"User-Agent": get_ua()})
    return parse_question_html(resp.text, index)

# ============ 导出 ============
def export_to_markdown(course_name: str, works: list[WorkItem], output_dir: str):
    """导出为 Markdown 文件"""
    os.makedirs(output_dir, exist_ok=True)
    safe_name = re.sub(r'[\\/:*?"<>|]', '_', course_name)
    filepath = os.path.join(output_dir, f"{safe_name}.md")
    
    lines = [f"# {course_name} — 作业题库\n"]
    lines.append(f"> 导出时间: {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
    
    for work in works:
        lines.append(f"## 📝 {work.name}")
        lines.append(f"**状态**: {work.status}  |  **题目数**: {len(work.questions)}\n")
        
        for q in work.questions:
            lines.append(f"### 第{q.index+1}题 — {q.qtype}")
            lines.append(f"**题目**: {q.content}\n")
            
            if q.options:
                lines.append("**选项**:")
                for opt in q.options:
                    lines.append(f"- {opt}")
                lines.append("")
            
            lines.append("---\n")
        
        lines.append("")
    
    with open(filepath, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))
    
    print(f"💾 已保存: {filepath}")
    return filepath

def export_to_json(course_name: str, works: list[WorkItem], output_dir: str):
    """导出为 JSON 文件"""
    os.makedirs(output_dir, exist_ok=True)
    safe_name = re.sub(r'[\\/:*?"<>|]', '_', course_name)
    filepath = os.path.join(output_dir, f"{safe_name}.json")
    
    data = {
        "course": course_name,
        "export_time": time.strftime('%Y-%m-%d %H:%M:%S'),
        "works": []
    }
    for w in works:
        data["works"].append({
            "name": w.name,
            "status": w.status,
            "questions": [asdict(q) for q in w.questions]
        })
    
    with open(filepath, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    
    print(f"💾 JSON 已保存: {filepath}")
    return filepath

# ============ 主流程 ============
def main():
    import urllib3
    urllib3.disable_warnings()
    
    print("=" * 60)
    print("📖 学习通作业题目摘取器")
    print("=" * 60)
    
    # 1. 登录
    print("\n🔑 正在登录...")
    session = login()
    
    # 2. 拉课程
    print("\n📚 正在拉取课程列表...")
    courses = get_courses(session)
    
    # 过滤目标课程
    if TARGET_COURSES:
        courses = [c for c in courses if any(t in c['name'] for t in TARGET_COURSES)]
        print(f"🎯 过滤后: {len(courses)} 门课程")
    
    all_files = []
    
    # 3. 逐课程处理
    for ci, course in enumerate(courses):
        print(f"\n{'='*40}")
        print(f"📖 [{ci+1}/{len(courses)}] {course['name']}")
        
        # 拉作业列表
        works_raw = get_work_list(session, course)
        print(f"   📋 作业列表: {len(works_raw)} 个")
        
        work_items = []
        
        for wi, w in enumerate(works_raw):
            print(f"   [{wi+1}/{len(works_raw)}] {w['name']} ({w['status']})")
            
            # 进入作业
            enter_info = enter_work(session, w)
            if not enter_info:
                print(f"      ⚠️ 无法进入（可能已过期或不可访问）")
                continue
            
            print(f"      题目数: {enter_info['questionTotal']}")
            
            # 获取试卷参数
            paper = get_work_paper(session, w, enter_info)
            if not paper or not paper.get("enc"):
                print(f"      ⚠️ 获取试卷参数失败")
                continue
            
            # 补充参数
            paper["cpi"] = enter_info["cpi"]
            paper["answerId"] = enter_info["answerId"]
            
            # 逐题拉取
            work_item = WorkItem(name=w['name'], status=w['status'], course_name=course['name'])
            
            for qi in range(enter_info["questionTotal"]):
                try:
                    q = get_question(session, w, paper, qi)
                    if q:
                        work_item.questions.append(q)
                        print(f"      第{qi+1}题: {q.qtype} — {q.content[:40]}...")
                    else:
                        print(f"      第{qi+1}题: ⚠️ 解析失败")
                except Exception as e:
                    print(f"      第{qi+1}题: ❌ {e}")
                
                time.sleep(0.5)  # 避免请求过快
            
            work_items.append(work_item)
            time.sleep(1)
        
        if work_items:
            # 导出
            md_path = export_to_markdown(course['name'], work_items, OUTPUT_DIR)
            json_path = export_to_json(course['name'], work_items, OUTPUT_DIR)
            all_files.extend([md_path, json_path])
    
    # 总结
    print("\n" + "=" * 60)
    print(f"✅ 完成！共处理 {len(courses)} 门课程")
    print(f"📁 输出目录: {OUTPUT_DIR}")
    for f in all_files:
        print(f"   📄 {f}")
    print("=" * 60)

if __name__ == "__main__":
    main()
