import requests
import os

API_KEY = "AIzaSyAoGR8ZqHgLHHVtOmNtBezVr-VtoBUCPw0"
URL = f"https://generativelanguage.googleapis.com/v1beta/models?key={API_KEY}"

def list_models():
    try:
        response = requests.get(URL)
        if response.status_code == 200:
            models = response.json()
            print("Available Models:")
            for model in models.get('models', []):
                if 'generateContent' in model.get('supportedGenerationMethods', []):
                    print(f"- {model['name']}")
        else:
            print(f"Error listing models: {response.status_code} - {response.text}")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == "__main__":
    list_models()
