import json
import requests
from models import ExtractionSchema

GEMINI_API_URL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent"

def generate_xpath_config(html_content: str, schema: dict, api_key: str) -> dict:
    """
    Generates XPath configuration using Gemini API.
    """
    
    prompt = f"""
    You are an expert in web scraping and XPath.
    I will provide you with an HTML snippet and an extraction schema.
    Your task is to generate a JSON configuration that maps the schema fields to XPaths in the HTML.
    
    ### GUIDELINES:
    1. **Robust XPaths**: Avoid brittle paths like `/html/body/div[1]/div[3]`. Use attributes like `@id`, `@class`, `@data-testid`, `@role`, or text content if unique (e.g., `//h2[contains(text(), 'Title')]`).
    2. **Arrays & Lists**: 
       - Identify the *repeating container* element for the array.
       - The `xpath` for the array field must select these container elements.
    3. **Relative Paths (CRITICAL)**:
       - For fields INSIDE an array/object, the `xpath` MUST be **relative** to the container.
       - Relative paths MUST start with `.` (e.g., `.//span[@class='price']` or `./div/text()`).
    4. **Text Extraction**:
       - Append `/text()` to extract text content directly if needed, or select the element if the extractor handles it.
       - For links, use `/@href`.
       - For images, use `/@src`.

    Input HTML:
    ```html
    {html_content}
    ```
    
    Extraction Schema:
    ```json
    {json.dumps(schema, indent=2)}
    ```
    
    Output Format:
    Return ONLY a valid JSON object. 
    The JSON should mirror the structure of the schema but with 'xpath' fields added.
    
    Example Output Structure:
    {{
        "name": "products",
        "xpath": "//div[contains(@class, 'product-card')]", // Container XPath
        "type": "array",
        "data": [
            {{
                "name": "title",
                "xpath": ".//h2/text()", // Relative to product-card
                "type": "string"
            }},
            {{
                "name": "url",
                "xpath": ".//a/@href", // Relative
                "type": "string"
            }}
        ]
    }}
    """

    payload = {
        "contents": [{
            "parts": [{"text": prompt}]
        }],
        "generationConfig": {
            "responseMimeType": "application/json"
        }
    }

    headers = {
        "Content-Type": "application/json"
    }

    response = requests.post(
        f"{GEMINI_API_URL}?key={api_key}",
        headers=headers,
        json=payload
    )

    if response.status_code != 200:
        raise Exception(f"Gemini API Error: {response.status_code} - {response.text}")

    try:
        result = response.json()
        text_response = result['candidates'][0]['content']['parts'][0]['text']
        
        # Clean up potential markdown formatting
        text_response = text_response.replace("```json", "").replace("```", "").strip()
        
        return json.loads(text_response)
    except (KeyError, json.JSONDecodeError) as e:
        raise Exception(f"Failed to parse Gemini response: {e}. Response: {response.text}")
