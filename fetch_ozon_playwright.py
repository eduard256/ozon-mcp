#!/usr/bin/env python3
"""Script to fetch HTML page from ozon.ru using Playwright"""

from playwright.sync_api import sync_playwright
import sys


def fetch_page(url: str = "https://www.ozon.ru", output_file: str = "page.html") -> None:
    """
    Fetch HTML content from URL using headless browser

    Args:
        url: URL to fetch
        output_file: File to save HTML content
    """
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(
            viewport={"width": 1920, "height": 1080},
            user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            locale="ru-RU",
        )
        page = context.new_page()

        print(f"Fetching {url}...")
        page.goto(url, wait_until="networkidle", timeout=60000)

        # Wait for content to load
        page.wait_for_timeout(3000)

        html = page.content()

        with open(output_file, "w", encoding="utf-8") as f:
            f.write(html)

        print(f"Saved to {output_file} ({len(html)} bytes)")

        browser.close()


if __name__ == "__main__":
    url = sys.argv[1] if len(sys.argv) > 1 else "https://www.ozon.ru"
    output = sys.argv[2] if len(sys.argv) > 2 else "/tmp/ozon.html"
    fetch_page(url, output)
