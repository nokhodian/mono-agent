import requests
import json
import os

BASE_URL = "http://localhost:8000"
INPUT_JSON_PATH = "linkedinsearchsample input.json"
HTML_PATH = "linkedinsearch.html"
OUTPUT_RESULT_PATH = "linkedinsearchSampleResult.json"

def test_system():
    # 1. Read input files
    print("Reading input files...")
    with open(INPUT_JSON_PATH, 'r') as f:
        input_data = json.load(f)
    
    with open(HTML_PATH, 'r') as f:
        html_content = f.read()
        
    # 2. Generate Config
    print("\n--- Testing /generate-config ---")
    # Use the sample HTML from the JSON input to avoid token limits/429 errors during testing
    sample_html = input_data["htmlContent"]
    
    payload_gen = {
        "htmlContent": sample_html,
        "purpose": input_data["purpose"],
        "extractionSchema": input_data["extractionSchema"]
    }
    
    try:
        response_gen = requests.post(f"{BASE_URL}/generate-config", json=payload_gen)
        if response_gen.status_code == 200:
            config = response_gen.json()
            print("Config generated successfully!")
            print(json.dumps(config, indent=2)[:500] + "...") # Print first 500 chars
        else:
            print(f"Failed to generate config: {response_gen.status_code} - {response_gen.text}")
            return
    except Exception as e:
        print(f"Error calling /generate-config: {e}")
        return

    # 3. Extract Data
    print("\n--- Testing /extract ---")
    payload_extract = {
        "htmlContent": sample_html,
        "config": config
    }
    
    try:
        response_extract = requests.post(f"{BASE_URL}/extract", json=payload_extract)
        if response_extract.status_code == 200:
            extracted_data = response_extract.json()
            print("Data extracted successfully!")
            print(json.dumps(extracted_data, indent=2)[:500] + "...") # Print first 500 chars
            
            # Compare with sample result (basic check)
            print(f"\nExtracted {len(extracted_data)} items.")
        else:
            print(f"Failed to extract data: {response_extract.status_code} - {response_extract.text}")
    except Exception as e:
        print(f"Error calling /extract: {e}")

if __name__ == "__main__":
    test_system()
