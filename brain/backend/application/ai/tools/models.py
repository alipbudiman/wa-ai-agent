from typing import Any, Literal, Optional, Union
from uuid import uuid4

from pydantic import BaseModel, ConfigDict, Field, model_validator
from enum import Enum

class LevelSystem(str, Enum):
    DEBUG = "DEBUG"
    INFO = "INFO"
    WARNING = "WARNING"
    ERROR = "ERROR"
    HISTORY = "HISTORY"

class GetUserInformationData(BaseModel):
    Found: bool
    FirstName: str | None = None
    FullName: str | None = None
    PushName: str | None = None
    BusinessName: str | None = None
    RedactedPhone: str | None = None

class GetUserInformationResponse(BaseModel):
    success: bool
    message: str
    data: GetUserInformationData | None = None

class SendMessageResponse(BaseModel):
    success: bool
    message: str
    data: Any | None = None
    status: str = "sent"
    on_hold: bool = False
    
class LogSystemEntry(BaseModel):
    timestamp: str = Field(description="ISO 8601 format timestamp of the log entry.")
    level: LevelSystem = Field(description="Log level indicating the severity or type of the log entry.")
    message: str = Field(description="Main log message content describing the event or action.")
    metadata: dict | None = None

class InputHistory(BaseModel):
    session_id: str = Field(description="Unique identifier for the input history entry.")
    timestamp: str = Field(description="format YYYY-MM-DD HH:MM:SS (human-readable time). e.g., 2026-04-11 14:35:22")
    sender: str = Field(description="Identifier of the user who sent the input.")
    message: str = Field(description="Content of the function that was called.")
    func_name: str = Field(description="Function name of the tool that was called.")

class SendMessageInput(BaseModel):
    number: str = Field(description="Recipient identifier. Required. Use a clean numeric user ID (e.g., 6281234567890) or a full WhatsApp JID when server must be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value.")
    message: str = Field(description="Message content to send. Required.")

class BroadcastToAdminsInput(BaseModel):
    message: str = Field(
        description=(
            "Short escalation message for admins. Include the user's request, issue, "
            "or reason they want to talk to a human/admin/CS. Do not include hidden reasoning."
        )
    )

class SendFileInput(BaseModel):
    number: str = Field(
        description=(
            "Recipient identifier. Required. Use a clean numeric user ID "
            "(e.g., 6281234567890) or a full WhatsApp JID when server must "
            "be preserved (e.g., 867051314767696@lid). Never guess or fabricate this value."
        )
    )
    file_path: str = Field(
        description=(
            "Absolute or relative path to the file to send. Required. "
            "The file MUST be located inside the 'playground/' directory. "
            "Always compress the file/folder first using the 'compress' tool before sending. "
            "Pass the resulting compressed file path here."
        )
    )
    caption: str | None = Field(
        default=None,
        description="Optional caption or description for the file being sent."
    )

class CommandInput(BaseModel):
    command: str = Field(description="Command to execute in the terminal. Required.")

class CommandInputGenerateImage(BaseModel):
    prompts: list[str] = Field(description="A list of text descriptions/prompts for image generation. Required. If multiple images are requested, you MUST provide multiple separate items in this list. DO NOT combine them into a single 'collage' prompt.")
    folder_path: str = Field(description="The absolute or relative path to the directory where images should be saved. Required.")

class WorkerTaskInput(BaseModel):
    task: str = Field(
        description=(
            "Canonical worker brief from the latest user request. Required. "
            "Start with the user's actual intent (same language, key entities, and constraints), "
            "then add steps/success criteria if needed. Do not replace user intent with internal "
            "diagnostic or test goals unless the user explicitly asks for testing."
        )
    )
    session_id: str | None = Field(
        default=None,
        description=(
            "Session ID from a previous worker execution. Optional. Defaults to None (new session). "
            "When the user wants to continue or resume a previous task, first call `agent_input_log` "
            "with `func_name='run_worker'` to retrieve existing sessions, then pass the matching "
            "`session_id` here so the worker resumes from prior conversation context. "
            "If no previous session exists or the task is entirely new, leave as None."
        )
    )
    work_dir: str | None = Field(
        default="playground/",
        description=(
            "Working directory for the agent's terminal commands. Optional. Defaults to 'playground/'. "
            "The agent should check existence before creating, and never create outside 'playground/' unless explicitly requested."
        ),
    )
    wait_for_completion: bool = Field(
        default=True,
        description=(
            "When true (default), wait for worker completion and return worker output in the same turn so "
            "the assistant can continue reasoning with that context. Set false for fire-and-forget background mode."
        ),
    )

class WebSearchInput(BaseModel):
    query: str = Field(
        description="Web search question or keyword from the current chat context. Required."
    )
    topic: str = Field(
        default="general",
        description=(
            "Tavily search topic. Optional. Use 'general' by default; use 'news' when the "
            "request is explicitly about recent/current events."
        ),
    )

class ToolDataCode(BaseModel):
    package: list[str] = Field(
        default_factory=list,
        description="List of Python packages required to execute the code. Optional. If the code requires specific packages, list them here so the system can ensure they are installed before execution. python packages should be listed by their PyPI name (e.g., 'numpy', 'requests'). If no special packages are needed, this can be an empty list or omitted."
    )
    code: str = Field(description="The actual code snippet to execute. Required. python code as a string. If the code is multi-line, ensure it is properly formatted with newline characters (\\n) to preserve structure when executed.")

class ToolData(BaseModel):
    name: str = Field(description="Name of the tool. Required.")
    description: str = Field(description="Description of the tool's functionality and purpose. Required.")
    agent_type: str = Field(description="Type of agent this tool is designed for (e.g., 'main-agent', 'code-agent'). Required.")
    code: ToolDataCode | None = Field(
        default=None,
        description="Code-related data for the tool. Optional. This can include the code snippet and required packages if the tool requires code execution."
    )
    information: str | None = Field(
        default=None,
        description=(
            "Additional information about the tool. Optional. This can include usage tips, constraints, or any other relevant details that don't fit in the description."
        ),
    )

    @model_validator(mode="after")
    def require_information_or_code(self):
        has_information = bool((self.information or "").strip())
        has_code = self.code is not None and bool((self.code.code or "").strip())
        if not has_information and not has_code:
            raise ValueError("ToolData requires at least one of information or code.")
        return self

class ToolAccess(BaseModel):
    tool_name: str = Field(description="Name of the custom database tool to access. Required. Use one of the names from the custom tools catalog.")
    tool_input: str | None = Field(
        default=None,
        description=(
            "Optional input for the selected custom tool. Use plain text or a JSON string when the custom tool needs user-specific data. "
            "For information-only tools, leave this empty."
        ),
    )

# Backward-compatible alias for the previous misspelled class name.
ToolAcccess = ToolAccess
    
class ToolDataRequest(BaseModel):
    agent_type: str

class ToolDataReturn(BaseModel):
    name: str
    description: str

# catalog

class WatchDetails(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
        json_schema_extra={
            "examples": [
                {
                    "movement_type": "automatic",
                    "material": "stainless steel",
                    "water_resistance_atm": 10,
                    "length_mm": 46.3,
                    "width_mm": 40.0,
                    "thickness_mm": 10.9,
                }
            ]
        }
    )

    movement_type: str = Field(description="Watch movement type, for example automatic, quartz, or mechanical.")
    material: str = Field(description="Main watch case or strap material.")
    water_resistance_atm: int = Field(description="Water resistance rating in ATM.")
    length_mm: float = Field(..., description="Lug-to-lug length in mm (e.g., 46.3).")
    width_mm: float = Field(..., description="Diameter/width in mm (e.g., 40.0).")
    thickness_mm: float = Field(..., description="Casing thickness in mm (e.g., 10.9).")

class ClothingDetails(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
        json_schema_extra={
            "examples": [
                {
                    "size": "L",
                    "fabric": "cotton",
                    "color": "black",
                }
            ]
        }
    )

    size: str = Field(description="Clothing size, for example S, M, L, XL, or a numeric size.")
    fabric: str = Field(description="Main fabric or material.")
    color: str = Field(description="Primary product color.")

class FurnitureDetails(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
        json_schema_extra={
            "examples": [
                {
                    "dimensions_cm": "120 x 60 x 75",
                    "primary_material": "solid wood",
                    "requires_assembly": True,
                }
            ]
        }
    )

    dimensions_cm: str = Field(description="Furniture dimensions in centimeters, for example '120 x 60 x 75'.")
    primary_material: str = Field(description="Primary furniture material.")
    requires_assembly: bool = Field(description="Whether the product requires assembly.")

class OtherDetails(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
        json_schema_extra={
            "examples": [
                {}
            ]
        }
    )

class ImageDetails(BaseModel):
    model_config = ConfigDict(
        extra="ignore",
        json_schema_extra={
            "examples": [
                {
                    "id": "5b1f8e61f2cf4d99a7df87971f5b0af6",
                    "url": "https://cdn.example.com/catalog/watch-001/front.webp",
                }
            ]
        },
    )

    id: str = Field(
        default_factory=lambda: uuid4().hex,
        description="Unique image id inside a catalog product.",
    )
    url: str = Field(description="Public URL to the product image.")

    @model_validator(mode="before")
    @classmethod
    def normalize_legacy_image_fields(cls, data: Any):
        if isinstance(data, str):
            return {"url": data}

        if not isinstance(data, dict):
            return data

        return {
            "id": data.get("id") or data.get("image_id") or uuid4().hex,
            "url": data.get("url") or data.get("image_url") or "",
        }

    @model_validator(mode="after")
    def validate_image_fields(self):
        self.id = self.id.strip() or uuid4().hex
        self.url = self.url.strip()
        if not self.url:
            raise ValueError("image url cannot be empty")

        return self

class CatalogProduct(BaseModel):
    model_config = ConfigDict(
        json_schema_extra={
            "examples": [
                {
                    "sku": "WATCH-SEIKO-001",
                    "product_name": "Seiko 5 Sports Automatic",
                    "description": "Automatic stainless steel daily watch.",
                    "price": 3250000,
                    "stock": 12,
                    "brand": "Seiko",
                    "shipping_weight_grams": 600,
                    "images": [
                        {
                            "id": "watch-front",
                            "url": "https://cdn.example.com/catalog/watch-001/front.webp",
                        }
                    ],
                    "category": "watch",
                    "details": {
                        "movement_type": "automatic",
                        "material": "stainless steel",
                        "water_resistance_atm": 10,
                        "length_mm": 46.3,
                        "width_mm": 40.0,
                        "thickness_mm": 10.9,
                    },
                },
                {
                    "sku": "TSHIRT-BASIC-001",
                    "product_name": "Basic Cotton T-Shirt",
                    "description": "Regular fit cotton t-shirt.",
                    "price": 149000,
                    "stock": 50,
                    "brand": "Dailywear",
                    "shipping_weight_grams": 250,
                    "images": [],
                    "category": "clothing",
                    "details": {
                        "size": "L",
                        "fabric": "cotton",
                        "color": "black",
                    },
                },
                {
                    "sku": "DESK-OAK-001",
                    "product_name": "Oak Working Desk",
                    "description": "Minimal working desk for home office.",
                    "price": 1750000,
                    "stock": 8,
                    "brand": "Homecraft",
                    "shipping_weight_grams": 18000,
                    "images": [],
                    "category": "furniture",
                    "details": {
                        "dimensions_cm": "120 x 60 x 75",
                        "primary_material": "solid wood",
                        "requires_assembly": True,
                    },
                },
                {
                    "sku": "GENERAL-001",
                    "product_name": "General Product",
                    "description": "Catalog product without category-specific details.",
                    "price": 99000,
                    "stock": 5,
                    "brand": "Generic",
                    "shipping_weight_grams": 500,
                    "images": [],
                    "category": "lainnya",
                    "details": {},
                },
            ]
        }
    )

    id: str = Field(
        default_factory=lambda: uuid4().hex,
        description="Unique catalog id. Optional on create; backend will generate one when omitted or empty.",
    )
    images: list[ImageDetails] = Field(
        default_factory=list,
        description="List of product images. Use image upload/delete endpoints to manage files safely.",
    )
    sku: str = Field(description="Stock keeping unit.")
    product_name: str = Field(description="Display name of the product.")
    description: Optional[str] = Field(default=None, description="Short product description.")
    price: float = Field(description="Product price.")
    stock: int = Field(default=0, description="Available stock count.")
    brand: Optional[str] = Field(default=None, description="Product brand.")
    shipping_weight_grams: int = Field(description="Shipping weight in grams.")
    ai_instruction: Optional[str] = Field(
        default=None,
        description=(
            "Admin-defined instruction for the reply agent on how to present or use this specific "
            "product when its details are sent to the user (emphasis, conditions, tone, eligibility). "
            "Optional. This is internal guidance and must never be echoed verbatim to the user; "
            "it is not part of the product message that gets sent."
        ),
    )

    # Menentukan pilihan kategori yang valid
    category: Literal["watch", "clothing", "furniture", "lainnya"] = Field(
        description=(
            "Required product category. Use 'watch' with WatchDetails, "
            "'clothing' with ClothingDetails, 'furniture' with FurnitureDetails, "
            "or 'lainnya' with empty details."
        )
    )
    
    # Field 'details' akan otomatis mengikuti model yang sesuai di atas
    details: Union[WatchDetails, ClothingDetails, FurnitureDetails, OtherDetails] = Field(
        default_factory=OtherDetails,
        description=(
            "Category-specific detail object. It must match the selected category: "
            "WatchDetails for watch, ClothingDetails for clothing, FurnitureDetails for furniture, "
            "or an empty object for lainnya."
        )
    )

    @model_validator(mode="before")
    @classmethod
    def normalize_legacy_catalog_images(cls, data: Any):
        if not isinstance(data, dict):
            return data

        normalized = dict(data)
        if normalized.get("category") == "lainnya":
            normalized["details"] = {}
        if "images" not in normalized and "image_url" in normalized:
            legacy_image_url = normalized.get("image_url")
            if isinstance(legacy_image_url, list):
                normalized["images"] = legacy_image_url
            elif legacy_image_url:
                normalized["images"] = [legacy_image_url]
            else:
                normalized["images"] = []
        if normalized.get("images") is None:
            normalized["images"] = []

        return normalized

    @model_validator(mode="after")
    def validate_category_details(self):
        self.id = self.id.strip() or uuid4().hex
        expected_details = {
            "watch": WatchDetails,
            "clothing": ClothingDetails,
            "furniture": FurnitureDetails,
            "lainnya": OtherDetails,
        }
        expected_model = expected_details[self.category]

        if not isinstance(self.details, expected_model):
            raise ValueError(
                f"details must use {expected_model.__name__} when category is '{self.category}'"
            )

        return self
    
class SendCatalogArgs(BaseModel):
    product_ids: list[str] = Field(
        description=(
            "List of catalog product IDs to include in the message. Required. "
            "Only call this tool when the user explicitly requests catalog details, product images, "
            "or product information for specific products. If the user does not specify which products "
            "they are interested in, ask for clarification instead of calling this tool."
        )
    )

    @model_validator(mode="after")
    def normalize_product_ids(self):
        normalized_ids: list[str] = []
        seen_ids: set[str] = set()
        for product_id in self.product_ids:
            clean_id = (product_id or "").strip()
            if clean_id and clean_id not in seen_ids:
                normalized_ids.append(clean_id)
                seen_ids.add(clean_id)

        if not normalized_ids:
            raise ValueError("product_ids must contain at least one catalog product id")

        self.product_ids = normalized_ids
        return self

class AdminData(BaseModel):
    name: str = Field(description="Name of the admin data entry. Required.")
    number: str = Field(description="Phone number associated with the admin data. Required.")

class AdminDataOutput(BaseModel):
    parameter: Literal["admins"] = Field(
        default="admins",
        description="Database parameter key for admin configuration.",
    )
    numbers: list[AdminData] = Field(
        default_factory=list,
        description="Admin contacts stored in manager_bot_config.numbers.",
    )
    waiting_for_admins: list[str] = Field(
        default_factory=list,
        description="Phone numbers waiting for admin follow-up.",
    )
