#!/usr/bin/env python3
"""Tổng hợp số đo Trường đua từ log server.

Dùng:  docker logs dcity-app 2>&1 | python3 tools/race_telemetry.py
       python3 tools/race_telemetry.py dcity.log

Đọc các dòng `race-telemetry {json}` (mỗi người một bản mỗi trận, không có tên / IP), in bảng theo người chơi (uid)
và theo loại thiết bị: số trận, FPS (trung vị / p5), khung > 50 ms, % giật / khựng xe mình và xe người khác,
độ trễ hiển thị, RTT, gói hụt / hiệu chỉnh lớn.
"""
import json
import statistics
import sys


def read(stream):
    for line in stream:
        i = line.find("race-telemetry ")
        if i < 0:
            continue
        try:
            yield json.loads(line[i + len("race-telemetry "):].strip())
        except ValueError:
            continue


def med(rows, key):
    vals = [r.get(key, 0) for r in rows if isinstance(r.get(key), (int, float))]
    return statistics.median(vals) if vals else 0


def table(title, groups):
    print(f"\n{title}")
    head = f"{'':<18}{'trận':>5}{'fps':>7}{'fps p5':>8}{'>50ms':>7}{'giật mình%':>11}{'giật khác%':>11}{'khựng khác%':>12}{'trễ vẽ':>8}{'rtt':>6}{'rtt95':>7}{'jit':>6}{'hụt':>5}{'lớn':>5}"
    print(head)
    print("-" * len(head))
    for name, rows in sorted(groups.items(), key=lambda kv: -len(kv[1])):
        print(f"{str(name):<18}{len(rows):>5}{med(rows, 'fps50'):>7.1f}{med(rows, 'fps5'):>8.1f}{med(rows, 'f50'):>7.0f}"
              f"{med(rows, 'meJerk'):>11.2f}{med(rows, 'remJerk'):>11.2f}{med(rows, 'remStall'):>12.2f}{med(rows, 'lag50'):>8.0f}"
              f"{med(rows, 'rtt50'):>6.0f}{med(rows, 'rtt95'):>7.0f}{med(rows, 'jit'):>6.1f}{med(rows, 'gap'):>5.0f}{med(rows, 'big'):>5.0f}")


def main():
    src = open(sys.argv[1], encoding="utf-8", errors="replace") if len(sys.argv) > 1 else sys.stdin
    rows = list(read(src))
    if not rows:
        print("Không có dòng race-telemetry nào.")
        return
    by_user, by_dev = {}, {}
    for r in rows:
        by_user.setdefault(f"uid {r.get('uid')}", []).append(r)
        dev = {"m": "điện thoại", "d": "máy tính"}.get(r.get("dev"), "?")
        by_dev.setdefault(f"{dev}{' low' if r.get('low') else ''} {int(r.get('hz') or 0)}Hz", []).append(r)
    print(f"{len(rows)} bản số đo, {len({(r.get('room'), r.get('race')) for r in rows})} trận (giá trị trong bảng là trung vị)")
    table("Theo người chơi", by_user)
    table("Theo thiết bị", by_dev)


if __name__ == "__main__":
    main()
