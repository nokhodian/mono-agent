from lxml import html
from typing import Any, Dict, List, Union

def extract_data(html_content: str, config: Dict[str, Any]) -> Any:
    """
    Extracts data from HTML based on the provided configuration.
    """
    tree = html.fromstring(html_content)
    
    # Handle case where config is wrapped in 'fields' (matching schema structure)
    if "fields" in config and isinstance(config["fields"], dict) and "type" in config["fields"]:
        config = config["fields"]
        
    return _extract_recursive(tree, config)

def _extract_recursive(element, config: Dict[str, Any]) -> Any:
    field_type = config.get("type")
    xpath = config.get("xpath")
    
    if not xpath:
        return None

    if field_type == "array":
        # Find all container elements
        # If element is the root tree, use xpath directly. 
        # If element is a node, use relative xpath (ensure it starts with . if meant to be relative)
        
        # However, the config generator might return absolute paths even for containers.
        # Let's assume the xpath provided for the array container is relative to the current 'element' context
        # unless it's the top level.
        
        items = element.xpath(xpath)
        results = []
        
        if not config.get("data"):
            return results

        # The 'data' in schema for array is a list of fields. 
        # But usually for an array of objects, 'data' in schema is a list containing ONE object definition 
        # which represents the structure of items in the array.
        # Or it could be a list of fields if the array items are objects.
        
        # Let's look at the input sample:
        # "data": [ { "name": "name", ... }, { "name": "position", ... } ]
        # This implies the array items are objects with these fields.
        
        for item in items:
            item_data = {}
            for field_config in config["data"]:
                field_name = field_config["name"]
                item_data[field_name] = _extract_recursive(item, field_config)
            results.append(item_data)
            
        return results

    elif field_type == "string":
        # Extract text
        try:
            results = element.xpath(xpath)
            if not results:
                return None
            
            extracted_text = ""
            
            if isinstance(results, list):
                # If we get a list of results, we should probably join them if they are all text strings.
                # If they are elements, we usually want the text content of the first one or join them?
                # For simplicity and robustness:
                # 1. If it's a list of strings (from /text()), join them.
                # 2. If it's a list of elements, take the text_content() of the first one (or all?).
                # Let's try to be smart: join all text representations.
                
                texts = []
                for res in results:
                    if isinstance(res, str):
                        texts.append(res)
                    elif hasattr(res, 'text_content'):
                        texts.append(res.text_content())
                    else:
                        texts.append(str(res))
                
                extracted_text = " ".join(texts)
            else:
                # Single result (unlikely with lxml xpath usually returning list, but possible)
                if isinstance(results, str):
                    extracted_text = results
                elif hasattr(results, 'text_content'):
                    extracted_text = results.text_content()
                else:
                    extracted_text = str(results)
            
            # Normalize whitespace: remove newlines and collapse multiple spaces
            return " ".join(extracted_text.split())
                
        except Exception as e:
            print(f"Error extracting {config.get('name')}: {e}")
            return None
            
    return None

def count_fields_with_value(data: Any) -> int:
    """
    Recursively counts the number of fields that have a non-empty value.
    """
    count = 0
    if isinstance(data, dict):
        for value in data.values():
            count += count_fields_with_value(value)
    elif isinstance(data, list):
        for item in data:
            count += count_fields_with_value(item)
    elif isinstance(data, str):
        if data and data.strip():
            count = 1
    elif data is not None:
        # For other types like int, float, bool
        count = 1
        
    return count

