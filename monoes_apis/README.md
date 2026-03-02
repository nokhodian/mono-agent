# AI-Powered Web Scraper

This project is a FastAPI-based web scraping service that leverages Google's Gemini API to intelligently generate XPath configurations for extracting structured data from HTML content.

## Features

- **Intelligent Config Generation**: Uses Gemini 2.5 Flash to analyze HTML and generate XPath selectors based on a user-defined schema.
- **Structured Extraction**: Extracts data into clean JSON formats (arrays, objects, strings) using the generated configuration.
- **HTML Cleaning**: Automatically cleans input HTML to reduce token usage and improve processing speed.

## Prerequisites

- Python 3.8+
- A Google Gemini API Key

## Installation

1.  **Clone the repository:**

    ```bash
    git clone <repository-url>
    cd <repository-directory>
    ```

2.  **Create a virtual environment (recommended):**

    ```bash
    python -m venv venv
    source venv/bin/activate  # On Windows: venv\Scripts\activate
    ```

3.  **Install dependencies:**
    ```bash
    pip install fastapi uvicorn pydantic python-dotenv requests lxml
    ```

## Configuration

1.  Create a `.env` file in the root directory.
2.  Add your Gemini API key:
    ```env
    GEMINI_API_KEY=your_actual_api_key_here
    ```

## Running the Server

Start the development server:

```bash
uvicorn main:app --reload
```

The server will start at `http://0.0.0.0:8000`.

## API Documentation

### 1. Generate Configuration (`POST /generate-config`)

Generates an extraction configuration (XPaths) based on the provided HTML and schema, and saves it to a file.

**Request Body:**

```json
{
  "htmlContent": "<html>...</html>",
  "purpose": "Extract user profiles",
  "extractionSchema": {
    "fields": {
      "name": "profiles",
      "description": "List of user profiles",
      "type": "array",
      "data": [
        {
          "name": "full_name",
          "description": "Name of the user",
          "type": "string"
        },
        {
          "name": "job_title",
          "description": "Job title of the user",
          "type": "string"
        }
      ]
    }
  },
  "configName": "linkedin-profiles"
}
```

**Response:**

Returns the generated filename and the configuration object.

```json
{
  "configName": "linkedin-profiles-23-11-27-18-15.json",
  "config": { ... }
}
```

### 2. Extract Data (`POST /extract`)

Extracts data from HTML using a previously generated configuration file.

**Request Body:**

```json
{
  "htmlContent": "<html>...</html>",
  "configName": "linkedin-profiles-23-11-27-18-15.json"
}
```

**Response:**

Returns the extracted data in JSON format.

## Example Workflow

1.  **Input**: You have a raw HTML string from a website.
2.  **Generate Config**: Send the HTML, your desired JSON schema, and a `configName` to `/generate-config`. The system will generate the XPaths and save the config to a file in the `configs/` directory.
3.  **Extract**: Send the same HTML and the `configName` (returned from the previous step) to `/extract`.
4.  **Result**: You get a clean JSON array of the data.
