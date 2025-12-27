#!/usr/bin/env python3
"""Script to fetch HTML page from ozon.ru"""

import requests


def fetch_ozon_page(url: str = "https://www.ozon.ru") -> str:
    """
    Fetch HTML content from ozon.ru

    Args:
        url: URL to fetch, defaults to ozon.ru homepage

    Returns:
        HTML content as string
    """
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
        "Accept-Language": "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7",
    }

    response = requests.get(url, headers=headers, timeout=30)
    response.raise_for_status()

    return response.text


def main():
    """Main function to fetch and save ozon.ru page"""
    print("Fetching ozon.ru...")

    html = fetch_ozon_page()

    output_file = "ozon_page.html"
    with open(output_file, "w", encoding="utf-8") as f:
        f.write(html)

    print(f"Saved to {output_file} ({len(html)} bytes)")


if __name__ == "__main__":
    main()
