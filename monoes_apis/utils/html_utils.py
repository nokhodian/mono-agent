import re
from lxml import html

def clean_html(html_content: str) -> str:
    """
    Aggressively cleans HTML content to reduce token usage for AI processing.
    Removes:
    - Head, scripts, styles, svg, etc.
    - Comments
    - Unnecessary attributes (keeps id, class, href, src, etc.)
    """
    if not html_content:
        return ""

    try:
        # Parse HTML
        tree = html.fromstring(html_content)
    except Exception:
        return html_content

    # 1. Remove specific tags entirely (including content)
    tags_to_remove = [
        'head', 'script', 'style', 'svg', 'noscript', 'iframe', 
        'meta', 'link', 'header', 'footer', 'nav', 'aside'
    ]
    
    for tag in tags_to_remove:
        for element in tree.xpath(f'//{tag}'):
            element.getparent().remove(element)

    # 2. Remove comments
    for comment in tree.xpath('//comment()'):
        comment.getparent().remove(comment)

    # 3. Whitelist attributes that are useful for XPath identification
    allowed_attributes = {
        'id', 'class', 'name', 'href', 'src', 'title', 'alt', 
        'data-id', 'data-testid', 'itemprop', 'role', 'type', 'value'
    }

    # Iterate over all elements to clean attributes
    for element in tree.iter():
        # Get attributes to remove
        attrs_to_remove = [
            attr for attr in element.attrib 
            if attr not in allowed_attributes and not attr.startswith('data-')
        ]
        
        for attr in attrs_to_remove:
            del element.attrib[attr]
            
        # Optional: Remove empty elements that have no attributes and no text?
        # Be careful not to remove structural divs that might be parents.
        # if not element.attrib and not (element.text and element.text.strip()) and len(element) == 0:
        #     element.getparent().remove(element)

    # Convert back to string
    cleaned_str = html.tostring(tree, encoding='unicode', pretty_print=True)
    
    # Collapse whitespace
    cleaned_str = re.sub(r'\s+', ' ', cleaned_str).strip()
    
    return cleaned_str
