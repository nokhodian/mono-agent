from fastapi import FastAPI, HTTPException, Request, Query
from models import GenerateConfigRequest, ExtractRequest, ExtractTestRequest
from services.gemini_service import generate_xpath_config
from services.extractor_service import extract_data, count_fields_with_value
from utils.html_utils import clean_html
import os
from dotenv import load_dotenv
import json
from datetime import datetime

load_dotenv()

app = FastAPI(debug=os.getenv("DEBUG", "false").lower() == "true")


@app.middleware("http")
async def fix_double_slash(request: Request, call_next):
    if "//" in request.url.path:
        request.scope["path"] = request.url.path.replace("//", "/")
    response = await call_next(request)
    return response


GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")
if not GEMINI_API_KEY:
    raise ValueError("GEMINI_API_KEY not found in environment variables")


# ... imports ...

@app.post("/generate-config")
async def generate_config(request: GenerateConfigRequest):
    try:
        # Clean HTML to reduce token usage and noise
        cleaned_html = clean_html(request.htmlContent)
        
        # We pass the raw dict of the schema
        schema_dict = request.extractionSchema.dict()
        
        # The schema input has a root 'fields' key which contains the main object/array definition.
        # Our gemini service expects the schema structure.
        
        config = generate_xpath_config(cleaned_html, schema_dict, GEMINI_API_KEY)
        
        # Save config to file
        timestamp = datetime.now().strftime("%y-%m-%d-%H-%M")
        filename = f"{request.configName}-{timestamp}.json"
        
        configs_dir = "configs"
        if not os.path.exists(configs_dir):
            os.makedirs(configs_dir)
            
        file_path = os.path.join(configs_dir, filename)
        with open(file_path, "w") as f:
            json.dump(config, f, indent=2)
            
        return {"configName": filename, "config": config}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/extract")
async def extract(request: ExtractRequest):
    try:
        # Load config from file
        configs_dir = "configs"
        file_path = os.path.join(configs_dir, request.configName)
        
        if not os.path.exists(file_path):
             # Try adding .json if missing
            if not file_path.endswith(".json"):
                file_path += ".json"
            
            if not os.path.exists(file_path):
                raise HTTPException(status_code=404, detail=f"Config file '{request.configName}' not found")
        
        with open(file_path, "r") as f:
            config = json.load(f)
            
        data = extract_data(request.htmlContent, config)
        return data
    except HTTPException as he:
        raise he
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/extracttest")
async def extract_test(request: ExtractTestRequest):
    try:
        configs_dir = "configs"
        if not os.path.exists(configs_dir):
             return []
        
        # List all files
        all_files = [f for f in os.listdir(configs_dir) if f.endswith(".json")]
        
        # Filter by name (case insensitive? user didn't specify, assume contains)
        matching_files = [f for f in all_files if request.configName in f]
        
        # Sort by modification time (newest first) to get "last 10"
        full_paths = [(f, os.path.join(configs_dir, f)) for f in matching_files]
        full_paths.sort(key=lambda x: os.path.getmtime(x[1]), reverse=True)
        
        # Take top 10
        top_10 = full_paths[:10]
        
        results = []
        
        for filename, filepath in top_10:
            try:
                with open(filepath, "r") as f:
                    config = json.load(f)
                
                extracted_data = extract_data(request.htmlContent, config)
                fields_with_value = count_fields_with_value(extracted_data)
                
                results.append({
                    "configName": filename,
                    "fieldsWithValue": fields_with_value
                })
            except Exception as e:
                # Log error or handle gracefully
                print(f"Error processing {filename}: {e}")
                results.append({
                    "configName": filename,
                    "fieldsWithValue": 0,
                    "error": str(e)
                })
                
        return results
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/configs")
async def list_configs(
    filter: str = Query(None, description="Filter by config name"),
    page: int = Query(1, ge=1, description="Page number"),
    limit: int = Query(20, ge=1, le=100, description="Items per page")
):
    configs_dir = "configs"
    if not os.path.exists(configs_dir):
        return {"configs": [], "total": 0, "page": page, "limit": limit}
    
    files = [f for f in os.listdir(configs_dir) if f.endswith(".json")]
    
    if filter:
        files = [f for f in files if filter in f]
        
    # Sort files to ensure consistent pagination (newest first based on timestamp in name)
    files.sort(reverse=True)
    
    total = len(files)
    start = (page - 1) * limit
    end = start + limit
    
    paginated_files = files[start:end]
    
    return {
        "configs": paginated_files,
        "total": total,
        "page": page,
        "limit": limit
    }

@app.get("/configs/{config_name}")
async def get_config(config_name: str):
    configs_dir = "configs"
    
    # Basic security check
    if ".." in config_name or "/" in config_name:
         raise HTTPException(status_code=400, detail="Invalid config name")

    file_path = os.path.join(configs_dir, config_name)
    
    if not os.path.exists(file_path):
        # Try adding .json
        if not file_path.endswith(".json"):
            file_path += ".json"
            
        if not os.path.exists(file_path):
            raise HTTPException(status_code=404, detail="Config not found")
            
    try:
        with open(file_path, "r") as f:
            config = json.load(f)
        return config
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
