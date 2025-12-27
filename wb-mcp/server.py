#!/usr/bin/env python3
"""Wildberries MCP Server - allows AI to search and browse products on Wildberries"""

import json
import re
import time
from typing import Optional

from fastmcp import FastMCP
from playwright.sync_api import sync_playwright, Page, Browser

mcp = FastMCP(name="Wildberries")

# Global browser instance
_browser: Optional[Browser] = None
_page: Optional[Page] = None


def get_browser() -> tuple[Browser, Page]:
    """Get or create browser instance"""
    global _browser, _page

    if _browser is None:
        playwright = sync_playwright().start()
        _browser = playwright.chromium.launch(
            headless=True,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-setuid-sandbox",
                "--disable-dev-shm-usage",
            ]
        )
        context = _browser.new_context(
            viewport={"width": 1920, "height": 1080},
            user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            locale="ru-RU",
            timezone_id="Europe/Moscow",
        )
        context.add_init_script("""
            Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
        """)
        _page = context.new_page()

    return _browser, _page


def wait_for_antibot(page: Page, timeout: int = 30) -> bool:
    """Wait for antibot check to pass"""
    for _ in range(timeout):
        time.sleep(1)
        title = page.title()
        if "Почти готово" not in title and "Доступ ограничен" not in title:
            return True
    return False


def extract_products(page: Page) -> list[dict]:
    """Extract product cards from page"""
    products = []

    try:
        page.wait_for_selector("article.product-card", timeout=10000)
        time.sleep(2)
    except Exception:
        return products

    cards = page.query_selector_all("article.product-card")

    for card in cards[:50]:  # Limit to 50 products
        try:
            product = {}

            # Product ID
            nm_id = card.get_attribute("data-nm-id")
            if nm_id:
                product["id"] = nm_id
                product["url"] = f"https://www.wildberries.ru/catalog/{nm_id}/detail.aspx"

            # Name
            name_el = card.query_selector("span.product-card__name")
            if name_el:
                product["name"] = name_el.inner_text().strip()

            # Brand
            brand_el = card.query_selector("span.product-card__brand")
            if brand_el:
                product["brand"] = brand_el.inner_text().strip()

            # Price
            price_el = card.query_selector("ins.price__lower-price")
            if price_el:
                price_text = price_el.inner_text().strip()
                price_clean = re.sub(r"[^\d]", "", price_text)
                if price_clean:
                    product["price"] = int(price_clean)

            # Rating
            rating_el = card.query_selector("span.address-rate-mini")
            if rating_el:
                try:
                    product["rating"] = float(rating_el.inner_text().strip())
                except ValueError:
                    pass

            if product.get("id"):
                products.append(product)

        except Exception:
            continue

    return products


@mcp.tool
def wb_search(
    query: str,
    sort: str = "popular",
    limit: int = 20
) -> str:
    """
    Search products on Wildberries

    Args:
        query: Search query (e.g. "iphone 15", "кроссовки nike")
        sort: Sort order - "popular" (default), "rate" (by rating), "priceup" (price low to high), "pricedown" (price high to low), "newly" (newest)
        limit: Maximum number of products to return (default 20, max 50)

    Returns:
        JSON with list of products containing id, name, brand, price, rating, url
    """
    _, page = get_browser()

    limit = min(limit, 50)

    url = f"https://www.wildberries.ru/catalog/0/search.aspx?search={query}&sort={sort}"

    page.goto(url, wait_until="domcontentloaded", timeout=60000)

    if not wait_for_antibot(page):
        return json.dumps({"error": "Failed to pass antibot check"}, ensure_ascii=False)

    products = extract_products(page)[:limit]

    return json.dumps({
        "query": query,
        "sort": sort,
        "count": len(products),
        "products": products
    }, ensure_ascii=False, indent=2)


@mcp.tool
def wb_product(product_id: str) -> str:
    """
    Get detailed product information by ID

    Args:
        product_id: Wildberries product ID (e.g. "482257013")

    Returns:
        JSON with product details: name, brand, price, rating, reviews_count, description, seller, url
    """
    _, page = get_browser()

    url = f"https://www.wildberries.ru/catalog/{product_id}/detail.aspx"

    page.goto(url, wait_until="domcontentloaded", timeout=60000)

    if not wait_for_antibot(page):
        return json.dumps({"error": "Failed to pass antibot check"}, ensure_ascii=False)

    try:
        page.wait_for_selector(".product-page", timeout=10000)
        time.sleep(2)
    except Exception:
        pass

    product = {"id": product_id, "url": url}

    # Name
    try:
        name_el = page.query_selector("h1.product-page__title")
        if name_el:
            product["name"] = name_el.inner_text().strip()
    except Exception:
        pass

    # Brand
    try:
        brand_el = page.query_selector("a.product-page__header-brand")
        if brand_el:
            product["brand"] = brand_el.inner_text().strip()
    except Exception:
        pass

    # Price
    try:
        price_el = page.query_selector("ins.price-block__final-price")
        if price_el:
            price_text = price_el.inner_text().strip()
            price_clean = re.sub(r"[^\d]", "", price_text)
            if price_clean:
                product["price"] = int(price_clean)
    except Exception:
        pass

    # Old price
    try:
        old_price_el = page.query_selector("del.price-block__old-price")
        if old_price_el:
            old_price_text = old_price_el.inner_text().strip()
            old_price_clean = re.sub(r"[^\d]", "", old_price_text)
            if old_price_clean:
                product["old_price"] = int(old_price_clean)
    except Exception:
        pass

    # Rating
    try:
        rating_el = page.query_selector("span.product-review__rating")
        if rating_el:
            product["rating"] = float(rating_el.inner_text().strip())
    except Exception:
        pass

    # Reviews count
    try:
        reviews_el = page.query_selector("span.product-review__count-review")
        if reviews_el:
            reviews_text = reviews_el.inner_text().strip()
            reviews_clean = re.sub(r"[^\d]", "", reviews_text)
            if reviews_clean:
                product["reviews_count"] = int(reviews_clean)
    except Exception:
        pass

    # Seller
    try:
        seller_el = page.query_selector("a.seller-info__name")
        if seller_el:
            product["seller"] = seller_el.inner_text().strip()
    except Exception:
        pass

    return json.dumps(product, ensure_ascii=False, indent=2)


@mcp.tool
def wb_category(
    category_url: str,
    sort: str = "popular",
    limit: int = 20
) -> str:
    """
    Get products from a category page

    Args:
        category_url: Full URL or path of category (e.g. "https://www.wildberries.ru/catalog/elektronika/smartfony" or "/catalog/elektronika/smartfony")
        sort: Sort order - "popular", "rate", "priceup", "pricedown", "newly"
        limit: Maximum number of products (default 20, max 50)

    Returns:
        JSON with list of products
    """
    _, page = get_browser()

    limit = min(limit, 50)

    if not category_url.startswith("http"):
        category_url = f"https://www.wildberries.ru{category_url}"

    if "?" in category_url:
        url = f"{category_url}&sort={sort}"
    else:
        url = f"{category_url}?sort={sort}"

    page.goto(url, wait_until="domcontentloaded", timeout=60000)

    if not wait_for_antibot(page):
        return json.dumps({"error": "Failed to pass antibot check"}, ensure_ascii=False)

    products = extract_products(page)[:limit]

    return json.dumps({
        "category_url": category_url,
        "sort": sort,
        "count": len(products),
        "products": products
    }, ensure_ascii=False, indent=2)


@mcp.tool
def wb_reviews(
    product_id: str,
    limit: int = 10
) -> str:
    """
    Get product reviews

    Args:
        product_id: Wildberries product ID
        limit: Maximum number of reviews (default 10, max 30)

    Returns:
        JSON with list of reviews containing text, rating, author, date
    """
    _, page = get_browser()

    limit = min(limit, 30)

    url = f"https://www.wildberries.ru/catalog/{product_id}/feedbacks"

    page.goto(url, wait_until="domcontentloaded", timeout=60000)

    if not wait_for_antibot(page):
        return json.dumps({"error": "Failed to pass antibot check"}, ensure_ascii=False)

    try:
        page.wait_for_selector(".feedback", timeout=10000)
        time.sleep(2)
    except Exception:
        return json.dumps({
            "product_id": product_id,
            "count": 0,
            "reviews": []
        }, ensure_ascii=False, indent=2)

    reviews = []
    review_els = page.query_selector_all(".feedback")

    for review_el in review_els[:limit]:
        try:
            review = {}

            # Text
            text_el = review_el.query_selector(".feedback__text")
            if text_el:
                review["text"] = text_el.inner_text().strip()

            # Rating
            stars = review_el.query_selector_all(".feedback__rating svg.active")
            review["rating"] = len(stars) if stars else None

            # Author
            author_el = review_el.query_selector(".feedback__header-author")
            if author_el:
                review["author"] = author_el.inner_text().strip()

            # Date
            date_el = review_el.query_selector(".feedback__date")
            if date_el:
                review["date"] = date_el.inner_text().strip()

            if review.get("text"):
                reviews.append(review)

        except Exception:
            continue

    return json.dumps({
        "product_id": product_id,
        "count": len(reviews),
        "reviews": reviews
    }, ensure_ascii=False, indent=2)


@mcp.tool
def wb_sellers(product_id: str) -> str:
    """
    Get all sellers for a product with their prices

    Args:
        product_id: Wildberries product ID

    Returns:
        JSON with list of sellers containing name, price, rating
    """
    _, page = get_browser()

    url = f"https://www.wildberries.ru/catalog/{product_id}/detail.aspx"

    page.goto(url, wait_until="domcontentloaded", timeout=60000)

    if not wait_for_antibot(page):
        return json.dumps({"error": "Failed to pass antibot check"}, ensure_ascii=False)

    try:
        page.wait_for_selector(".product-page", timeout=10000)
        time.sleep(2)
    except Exception:
        pass

    # Try to find "all sellers" button and click it
    try:
        sellers_btn = page.query_selector("button.seller-info__more")
        if sellers_btn:
            sellers_btn.click()
            time.sleep(2)
    except Exception:
        pass

    sellers = []

    # Get main seller
    try:
        main_seller = {}
        seller_el = page.query_selector("a.seller-info__name")
        if seller_el:
            main_seller["name"] = seller_el.inner_text().strip()

        price_el = page.query_selector("ins.price-block__final-price")
        if price_el:
            price_text = price_el.inner_text().strip()
            price_clean = re.sub(r"[^\d]", "", price_text)
            if price_clean:
                main_seller["price"] = int(price_clean)

        if main_seller.get("name"):
            sellers.append(main_seller)
    except Exception:
        pass

    # Get other sellers from modal if opened
    try:
        seller_items = page.query_selector_all(".sellers-list__item")
        for item in seller_items:
            seller = {}

            name_el = item.query_selector(".seller-name")
            if name_el:
                seller["name"] = name_el.inner_text().strip()

            price_el = item.query_selector(".price")
            if price_el:
                price_text = price_el.inner_text().strip()
                price_clean = re.sub(r"[^\d]", "", price_text)
                if price_clean:
                    seller["price"] = int(price_clean)

            if seller.get("name") and seller not in sellers:
                sellers.append(seller)
    except Exception:
        pass

    return json.dumps({
        "product_id": product_id,
        "count": len(sellers),
        "sellers": sellers
    }, ensure_ascii=False, indent=2)


if __name__ == "__main__":
    mcp.run()
