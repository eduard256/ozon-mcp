#!/usr/bin/env python3
"""
Ozon Parser - uses real browser via Playwright to fetch product data
"""

import json
import sys
import time
import re
from playwright.sync_api import sync_playwright, Page, Browser


class OzonParser:
    def __init__(self, headless: bool = True, debug: bool = False):
        self.headless = headless
        self.debug = debug
        self.playwright = None
        self.browser = None
        self.context = None

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.stop()

    def start(self):
        """Start browser"""
        self.playwright = sync_playwright().start()

        # Launch real Chromium browser
        self.browser = self.playwright.chromium.launch(
            headless=self.headless,
            args=[
                '--no-sandbox',
                '--disable-setuid-sandbox',
                '--disable-dev-shm-usage',
                '--disable-blink-features=AutomationControlled',
            ]
        )

        # Create context with realistic settings
        self.context = self.browser.new_context(
            viewport={'width': 1920, 'height': 1080},
            user_agent='Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            locale='ru-RU',
            timezone_id='Europe/Moscow',
        )

        # Add stealth scripts
        self.context.add_init_script("""
            // Remove webdriver flag
            Object.defineProperty(navigator, 'webdriver', {
                get: () => undefined
            });

            // Mock plugins
            Object.defineProperty(navigator, 'plugins', {
                get: () => [1, 2, 3, 4, 5]
            });

            // Mock languages
            Object.defineProperty(navigator, 'languages', {
                get: () => ['ru-RU', 'ru', 'en-US', 'en']
            });

            // Mock permissions
            const originalQuery = window.navigator.permissions.query;
            window.navigator.permissions.query = (parameters) => (
                parameters.name === 'notifications' ?
                    Promise.resolve({ state: Notification.permission }) :
                    originalQuery(parameters)
            );

            // Mock chrome
            window.chrome = {
                runtime: {}
            };
        """)

        if self.debug:
            print("Browser started", file=sys.stderr)

    def stop(self):
        """Stop browser"""
        if self.context:
            self.context.close()
        if self.browser:
            self.browser.close()
        if self.playwright:
            self.playwright.stop()
        if self.debug:
            print("Browser stopped", file=sys.stderr)

    def _wait_for_page(self, page: Page, timeout: int = 30):
        """Wait for page to fully load and pass antibot"""
        start = time.time()

        while time.time() - start < timeout:
            title = page.title()
            html = page.content()

            # Check if we passed antibot
            if 'Доступ ограничен' not in html and 'Antibot' not in title:
                if self.debug:
                    print(f"Page loaded: {title}", file=sys.stderr)
                return True

            if self.debug:
                print(f"Waiting for antibot... ({int(time.time() - start)}s)", file=sys.stderr)

            time.sleep(1)

        return False

    def get_page_html(self, url: str) -> str:
        """Get raw HTML of page"""
        page = self.context.new_page()

        try:
            if self.debug:
                print(f"Opening: {url}", file=sys.stderr)

            page.goto(url, wait_until='domcontentloaded', timeout=60000)

            # Wait for page to load
            time.sleep(3)

            # Simulate human behavior
            page.mouse.move(500, 300)
            time.sleep(0.5)
            page.mouse.wheel(0, 300)
            time.sleep(1)

            # Wait for antibot to pass
            if not self._wait_for_page(page, timeout=30):
                if self.debug:
                    print("Failed to pass antibot", file=sys.stderr)

            return page.content()

        finally:
            page.close()

    def search(self, query: str, max_products: int = 10) -> dict:
        """Search for products"""
        url = f"https://www.ozon.ru/search/?text={query}&from_global=true"
        page = self.context.new_page()

        try:
            if self.debug:
                print(f"Searching: {query}", file=sys.stderr)

            page.goto(url, wait_until='domcontentloaded', timeout=60000)
            time.sleep(3)

            # Simulate scrolling
            for _ in range(3):
                page.mouse.wheel(0, 500)
                time.sleep(1)

            # Wait for antibot
            if not self._wait_for_page(page, timeout=30):
                html = page.content()
                with open('/tmp/ozon_debug.html', 'w') as f:
                    f.write(html)
                return {'query': query, 'count': 0, 'products': [], 'error': 'antibot_blocked'}

            # Additional scroll to load products
            for _ in range(3):
                page.mouse.wheel(0, 800)
                time.sleep(0.5)

            time.sleep(2)

            # Extract products
            products = []

            # Find all product links
            links = page.query_selector_all('a[href*="/product/"]')

            seen = set()
            for link in links:
                if len(products) >= max_products:
                    break

                try:
                    href = link.get_attribute('href')
                    if not href or '/product/' not in href:
                        continue

                    # Extract product ID to avoid duplicates
                    match = re.search(r'/product/[^/]+-(\d+)', href)
                    if not match:
                        continue

                    product_id = match.group(1)
                    if product_id in seen:
                        continue
                    seen.add(product_id)

                    # Get product info
                    full_url = f"https://www.ozon.ru{href}" if href.startswith('/') else href

                    # Try to get text content
                    text = link.inner_text() or ''
                    lines = [l.strip() for l in text.split('\n') if l.strip()]

                    name = ''
                    price = ''

                    for line in lines:
                        if '₽' in line and not price:
                            price = line
                        elif len(line) > 10 and not name and '₽' not in line:
                            name = line

                    # Get image
                    image = ''
                    img = link.query_selector('img')
                    if img:
                        image = img.get_attribute('src') or ''

                    products.append({
                        'name': name,
                        'price': price,
                        'link': full_url,
                        'image': image,
                        'id': product_id
                    })

                except Exception as e:
                    if self.debug:
                        print(f"Error parsing product: {e}", file=sys.stderr)
                    continue

            return {
                'query': query,
                'count': len(products),
                'products': products
            }

        finally:
            page.close()

    def get_product(self, url: str) -> dict:
        """Get product details"""
        page = self.context.new_page()

        try:
            if self.debug:
                print(f"Opening product: {url}", file=sys.stderr)

            page.goto(url, wait_until='domcontentloaded', timeout=60000)
            time.sleep(3)

            # Simulate human
            page.mouse.wheel(0, 300)
            time.sleep(1)

            if not self._wait_for_page(page, timeout=30):
                return {'error': 'antibot_blocked', 'url': url}

            product = {'url': url}

            # Get title
            h1 = page.query_selector('h1')
            if h1:
                product['name'] = h1.inner_text().strip()

            # Get price
            price_el = page.query_selector('[data-widget="webPrice"]')
            if price_el:
                product['price'] = price_el.inner_text().strip()

            # Get images
            images = []
            for img in page.query_selector_all('[data-widget="webGallery"] img'):
                src = img.get_attribute('src')
                if src:
                    images.append(src)
            product['images'] = images

            # Get rating
            rating_el = page.query_selector('[data-widget="webReviewProductScore"]')
            if rating_el:
                product['rating'] = rating_el.inner_text().strip()

            return product

        finally:
            page.close()

    def screenshot(self, url: str, path: str = '/tmp/screenshot.png') -> str:
        """Take screenshot of page"""
        page = self.context.new_page()

        try:
            page.goto(url, wait_until='domcontentloaded', timeout=60000)
            time.sleep(5)
            page.screenshot(path=path, full_page=True)
            return path
        finally:
            page.close()


def main():
    import argparse

    parser = argparse.ArgumentParser(description='Ozon Parser')
    parser.add_argument('command', choices=['search', 'product', 'html', 'screenshot'])
    parser.add_argument('query', help='Search query or URL')
    parser.add_argument('--max', type=int, default=10, help='Max products')
    parser.add_argument('--debug', action='store_true', help='Debug mode')
    parser.add_argument('--headed', action='store_true', help='Show browser')

    args = parser.parse_args()

    with OzonParser(headless=not args.headed, debug=args.debug) as ozon:
        if args.command == 'search':
            result = ozon.search(args.query, args.max)
            print(json.dumps(result, ensure_ascii=False, indent=2))

        elif args.command == 'product':
            result = ozon.get_product(args.query)
            print(json.dumps(result, ensure_ascii=False, indent=2))

        elif args.command == 'html':
            html = ozon.get_page_html(args.query)
            print(html)

        elif args.command == 'screenshot':
            path = ozon.screenshot(args.query)
            print(f"Screenshot saved to: {path}")


if __name__ == '__main__':
    main()
