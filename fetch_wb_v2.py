#!/usr/bin/env python3
"""Script to fetch HTML page from Wildberries using Playwright with anti-detection"""

from playwright.sync_api import sync_playwright
import sys
import time


def fetch_page(url: str, output_file: str = "/tmp/page.html") -> None:
    """
    Fetch HTML content from URL using headless browser with anti-detection

    Args:
        url: URL to fetch
        output_file: File to save HTML content
    """
    with sync_playwright() as p:
        browser = p.chromium.launch(
            headless=True,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-setuid-sandbox",
                "--disable-dev-shm-usage",
            ]
        )
        context = browser.new_context(
            viewport={"width": 1920, "height": 1080},
            user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            locale="ru-RU",
            timezone_id="Europe/Moscow",
        )

        # Remove webdriver flag
        context.add_init_script("""
            Object.defineProperty(navigator, 'webdriver', {
                get: () => undefined
            });
        """)

        page = context.new_page()

        print(f"Fetching {url}...")
        page.goto(url, wait_until="domcontentloaded", timeout=60000)

        # Wait for antibot to pass
        print("Waiting for antibot check...")
        for i in range(30):
            time.sleep(1)
            title = page.title()
            print(f"  [{i+1}s] Title: {title}")
            if "Почти готово" not in title and "Доступ ограничен" not in title:
                print("Antibot passed!")
                break

        # Additional wait for content
        page.wait_for_timeout(3000)

        html = page.content()

        with open(output_file, "w", encoding="utf-8") as f:
            f.write(html)

        print(f"Saved to {output_file} ({len(html)} bytes)")

        browser.close()


if __name__ == "__main__":
    url = sys.argv[1] if len(sys.argv) > 1 else "https://www.wildberries.ru"
    output = sys.argv[2] if len(sys.argv) > 2 else "/tmp/wb.html"
    fetch_page(url, output)
