#!/usr/bin/env python3
"""
AIM Benchmark Fixture Generator
================================
生成压测所需的 user.json 和 msg.txt 文件。

  user.json: 1000 条用户数据（email, password, username）
  msg.txt:   1000 条随机消息字符串（每行一条）

Usage:
  python generate_fixtures.py                # 默认 1000 条
  python generate_fixtures.py --count 5000   # 自定义数量
"""

import json
import random
import string
import uuid
import argparse
import os
import sys

# Force UTF-8 on Windows
if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass


def generate_users(count: int) -> list[dict]:
    """生成 count 条用户注册数据。"""
    users = []
    for i in range(count):
        uid = uuid.uuid4().hex[:8]
        users.append({
            "email": f"bench_{uid}_{i}@aim.dev",
            "password": "bench123456",
            "username": f"BenchUser_{i:04d}",
        })
    return users


def generate_messages(count: int) -> list[str]:
    """生成 count 条随机消息字符串。"""
    messages = []
    # 预定义一些有意义的消息模板，增加真实感
    templates = [
        "你好，在吗？",
        "今天天气不错",
        "午饭吃什么",
        "收到，我看看",
        "稍等一下",
        "好的，没问题",
        "明天几点出发",
        "那个文档发我一下",
        "周末有什么安排",
        "已阅，谢谢",
    ]

    for i in range(count):
        roll = random.random()
        if roll < 0.3:
            # 30%: 从模板中选
            msg = random.choice(templates)
        elif roll < 0.6:
            # 30%: 模板 + 随机数字
            msg = f"{random.choice(templates)} #{random.randint(1, 9999)}"
        elif roll < 0.85:
            # 25%: 纯随机字符串（8-64 字符）
            length = random.randint(8, 64)
            msg = "".join(random.choices(string.ascii_letters + string.digits, k=length))
        else:
            # 15%: 中文随机字符组合
            cn_chars = "的一是不了人我在有他这为之大来以个中上们到说时地也子就道会那要下看天与给年"
            length = random.randint(4, 20)
            msg = "".join(random.choices(cn_chars, k=length))
        messages.append(msg)

    return messages


def main():
    parser = argparse.ArgumentParser(
        description="AIM Benchmark Fixture Generator",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--count", type=int, default=1000,
        help="生成条数（默认 1000）",
    )
    parser.add_argument(
        "--output-dir", type=str, default="",
        help="输出目录（默认: 脚本所在目录）",
    )
    args = parser.parse_args()

    output_dir = args.output_dir or os.path.dirname(os.path.abspath(__file__))
    count = args.count

    # 生成 user.json
    users = generate_users(count)
    user_path = os.path.join(output_dir, "user.json")
    with open(user_path, "w", encoding="utf-8") as f:
        json.dump(users, f, indent=2, ensure_ascii=False)
    print(f"✓ {user_path}  ({len(users)} users)")

    # 生成 msg.txt
    messages = generate_messages(count)
    msg_path = os.path.join(output_dir, "msg.txt")
    with open(msg_path, "w", encoding="utf-8") as f:
        for msg in messages:
            f.write(msg + "\n")
    print(f"✓ {msg_path}  ({len(messages)} messages)")


if __name__ == "__main__":
    main()
