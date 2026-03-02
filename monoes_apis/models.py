from pydantic import BaseModel, Field
from typing import List, Dict, Any, Optional

class ExtractionField(BaseModel):
    name: str
    description: str
    type: str
    data: Optional[List['ExtractionField']] = None

class ExtractionSchema(BaseModel):
    fields: ExtractionField

class GenerateConfigRequest(BaseModel):
    htmlContent: str
    purpose: str
    extractionSchema: ExtractionSchema
    configName: str

class ExtractRequest(BaseModel):
    htmlContent: str
    configName: str

class ExtractTestRequest(BaseModel):
    htmlContent: str
    configName: str
