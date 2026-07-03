from backend.application.ingestion.loader import (
    DocumentDeleteResult,
    DocumentIngestResult,
    DocumentListResponse,
    DocumentLoader,
    SupportedDocumentType,
)
from datetime import datetime
import logging

from fastapi import APIRouter, BackgroundTasks, File, HTTPException, Query, UploadFile, status
from pydantic import BaseModel, Field

from backend.application.ai.agents import ChatAgentUseCase
from backend.application.ai.code import AgentCodeUsecase
from backend.application.ai.reaserch import ResearchAgentUsecase
from backend.application.health import CheckHealthUseCase
from backend.application.report import ReportUseCase
from backend.domain.report import LangSmithLogReport, LangSmithRunReport
from backend.application.ai.tools.models import AdminData, AdminDataOutput, CatalogProduct, ImageDetails, LevelSystem
from backend.application.aidata import CheckAIData
from backend.domain.aidata import AIDataInjectionResponse, AIDataStatus
from backend.application.ai.reply import ReplyAgentUsecase
from backend.application.ai.tools.catalog import Catalog
from backend.application.ai.tools.dbaccess import DBAcess
from backend.scoring.scoring import ScoringAgent

logger = logging.getLogger('uvicorn.error')


class HealthResponse(BaseModel):
    status: str
    service: str
    timestamp: datetime

class ChatAgentRequest(BaseModel):
    number: str
    message: str
    is_system: bool
    level: str = "INFO"
    group_id: str | None = None
    reply_content: str | None = None
    
class ChatAgentResponse(BaseModel):
    status: str
    response: str


class ChatAgentProcessDiagramResponse(BaseModel):
    status: str
    mermaid: str


class AIDataInjectionRequest(BaseModel):
    number: str
    messages: str


class VectorStoreIngestRequest(BaseModel):
    agent: str | None = None
    folder_path: str | None = None
    urls: list[str] | None = None
    file_paths: list[str] | None = None
    category: str = "general"


class VectorStoreDocumentStatusResponse(BaseModel):
    source: str
    exists: bool
    total_chunks: int


class CatalogPreviewItem(BaseModel):
    id: str = Field(description="Unique catalog product id.")
    product_name: str = Field(description="Display name of the product.")
    description: str | None = Field(default=None, description="Short product description.")
    price: float = Field(description="Product price.")


class CatalogDeleteResponse(BaseModel):
    id: str = Field(description="Deleted catalog product id.")
    deleted: bool = Field(description="True when a catalog product was deleted.")


class CatalogImageURLRequest(BaseModel):
    url: str = Field(description="Public image URL to attach to the catalog product.")


class AdminDeleteResponse(BaseModel):
    number: str = Field(description="Deleted admin phone number.")
    deleted: bool = Field(description="True when an admin was deleted.")


CATALOG_DETAILS_GUIDE = """
Panduan payload catalog:

- `category="watch"` wajib memakai `WatchDetails` pada `details`: `movement_type`, `material`, `water_resistance_atm`, `length_mm`, `width_mm`, `thickness_mm`.
- `category="clothing"` wajib memakai `ClothingDetails` pada `details`: `size`, `fabric`, `color`.
- `category="furniture"` wajib memakai `FurnitureDetails` pada `details`: `dimensions_cm`, `primary_material`, `requires_assembly`.
- `category="lainnya"` memakai `details: {}`. Backend akan menyimpan detail kosong untuk kategori ini.
- `category` wajib dipilih dari `watch`, `clothing`, `furniture`, atau `lainnya`. Request tanpa `category`, dengan `category` kosong, atau dengan bentuk `details` yang tidak cocok akan ditolak oleh validasi backend.
- `id` boleh tidak dikirim saat create; backend akan membuat id otomatis.
"""


REPLY_ADMIN_GUIDE = """
Reply admin adalah daftar admin untuk Reply Agent.

Ketika Reply Agent membutuhkan bantuan manusia, backend akan melakukan broadcast pesan ke nomor admin yang tersimpan di konfigurasi ini.

Payload admin:

- `name`: nama admin yang mudah dibaca di dashboard.
- `number`: nomor WhatsApp admin, contoh `6281234567890`.

Data disimpan mengikuti dokumen database `manager_bot_config` dengan bentuk:

- `parameter: "admins"`
- `numbers: AdminData[]`
- `waiting_for_admins: string[]`
"""


def create_health_router(usecase: CheckHealthUseCase) -> APIRouter:
    router = APIRouter(prefix="/checkhealth", tags=["health"])

    @router.get("", response_model=HealthResponse)
    def check_health() -> HealthResponse:
        result = usecase.execute()
        return HealthResponse(
            status=result.status,
            service=result.service,
            timestamp=result.timestamp,
        )

    return router

def create_ai_data_router(usecase: CheckAIData) -> APIRouter:
    router = APIRouter(prefix="/aidata", tags=["aidata"])

    @router.get("/providers", response_model=AIDataStatus)
    def check_ai_data() -> AIDataStatus:
        result = usecase.execute()
        return AIDataStatus(
            list_ai_providers=result.list_ai_providers,
        )
    
    @router.post("/inject-conversation", response_model=AIDataInjectionResponse)
    async def inject_conversation(request: AIDataInjectionRequest):
        return await usecase.injectAIConversation(number=request.number, messages=request.messages)

    return router

def create_langsmith_getlist_router(usecase: ReportUseCase) -> APIRouter:
    router = APIRouter(prefix="/langsmith", tags=["langsmith"])

    @router.get("/runs", response_model=list[LangSmithRunReport])
    def get_runs():
        return usecase.get_list()

    return router

def create_langsmith_select_run_router(usecase: ReportUseCase) -> APIRouter:
    router = APIRouter(prefix="/langsmith", tags=["langsmith"])

    @router.get("/runs/{run_id}", response_model=LangSmithLogReport)
    def select_run(run_id: str):
        return usecase.select_run(run_id)

    return router


def create_chat_agent_router(usecase: ChatAgentUseCase) -> APIRouter:
    router = APIRouter(prefix="/chat-agent", tags=["chat-agent"])

    @router.get("/process-diagram", response_model=ChatAgentProcessDiagramResponse)
    def get_process_diagram() -> ChatAgentProcessDiagramResponse:
        mermaid_source = usecase.get_process_diagram_mermaid().strip()
        if not mermaid_source:
            raise HTTPException(status_code=503, detail="process diagram is not available")

        return ChatAgentProcessDiagramResponse(
            status="success",
            mermaid=mermaid_source,
        )

    @router.post("", response_model=ChatAgentResponse)
    async def chat_agent(request: ChatAgentRequest, background_tasks: BackgroundTasks):
        try:
            level = LevelSystem[request.level.upper()]
        except KeyError:
            level = LevelSystem.INFO
            logger.info(f"Invalid log level '{request.level}' received. Defaulting to INFO.")
        background_tasks.add_task(
            usecase.ChatAgent,
            messages=request.message,
            number=request.number,
            is_system=request.is_system,
            level=level,
            group_id=request.group_id if request.group_id else None,
            reply_content=request.reply_content if request.reply_content else None,
            # image=request.image if request.image else None,
        )
        return ChatAgentResponse(
            status="success",
            response="success to call brain"
        )

    return router

def create_chat_agent_code_router(usecase: AgentCodeUsecase) -> APIRouter:
    router = APIRouter(prefix="/chat-agent-code", tags=["chat-agent-code"])

    @router.get("/process-diagram", response_model=ChatAgentProcessDiagramResponse)
    def get_code_process_diagram() -> ChatAgentProcessDiagramResponse:
        mermaid_source = usecase.get_process_diagram_mermaid().strip()
        if not mermaid_source:
            raise HTTPException(status_code=503, detail="process diagram is not available")
        return ChatAgentProcessDiagramResponse(status="success", mermaid=mermaid_source)

    @router.post("", response_model=ChatAgentResponse)
    async def chat_agent_code(
            request: ChatAgentRequest, 
            background_tasks: BackgroundTasks
        ) -> ChatAgentResponse:
        try:
            level = LevelSystem[request.level.upper()]
        except KeyError:
            level = LevelSystem.INFO
            logger.info(f"Invalid log level '{request.level}' received. Defaulting to INFO.")
        background_tasks.add_task(
            usecase.CodeChatAgent,
            messages=request.message,
            number=request.number,
            is_system=request.is_system,
            level=level,
            group_id=request.group_id if request.group_id else None,
            reply_content=request.reply_content if request.reply_content else None,
            # image=request.image if request.image else None,
        )
        return ChatAgentResponse(
            status="success",
            response="success to call brain"
        )

    return router

def create_chat_agent_research_router(usecase: ResearchAgentUsecase) -> APIRouter:
    router = APIRouter(prefix="/agent-research", tags=["agent-research"])

    @router.get("/process-diagram", response_model=ChatAgentProcessDiagramResponse)
    def get_research_process_diagram() -> ChatAgentProcessDiagramResponse:
        mermaid_source = usecase.get_process_diagram_mermaid().strip()
        if not mermaid_source:
            raise HTTPException(status_code=503, detail="process diagram is not available")
        return ChatAgentProcessDiagramResponse(status="success", mermaid=mermaid_source)

    @router.post("", response_model=ChatAgentResponse)
    async def agent_research(
            request: ChatAgentRequest, 
            background_tasks: BackgroundTasks
        ) -> ChatAgentResponse:
        try:
            level = LevelSystem[request.level.upper()]
        except KeyError:
            level = LevelSystem.INFO
            logger.info(f"Invalid log level '{request.level}' received. Defaulting to INFO.")
        background_tasks.add_task(
            usecase.ResearchAgent,
            messages=request.message,
            number=request.number,
            is_system=request.is_system,
            level=level,
            group_id=request.group_id if request.group_id else None,
            reply_content=request.reply_content if request.reply_content else None,
            # image=request.image if request.image else None,
        )
        return ChatAgentResponse(
            status="success",
            response="success to call brain"
        )

    return router

def create_chat_agent_reply_router(usecase: ReplyAgentUsecase) -> APIRouter:
    router = APIRouter(prefix="/agent-reply", tags=["agent-reply"])

    @router.get("/process-diagram", response_model=ChatAgentProcessDiagramResponse)
    def get_reply_process_diagram() -> ChatAgentProcessDiagramResponse:
        mermaid_source = usecase.get_process_diagram_mermaid().strip()
        if not mermaid_source:
            raise HTTPException(status_code=503, detail="process diagram is not available")
        return ChatAgentProcessDiagramResponse(status="success", mermaid=mermaid_source)

    @router.post("", response_model=ChatAgentResponse)
    async def agent_reply(
            request: ChatAgentRequest, 
            background_tasks: BackgroundTasks
        ) -> ChatAgentResponse:
        try:
            level = LevelSystem[request.level.upper()]
        except KeyError:
            level = LevelSystem.INFO
            logger.info(f"Invalid log level '{request.level}' received. Defaulting to INFO.")
        background_tasks.add_task(
            usecase.ReplyAgent,
            messages=request.message,
            number=request.number,
            is_system=request.is_system,
            level=level,
            group_id=request.group_id if request.group_id else None,
            reply_content=request.reply_content if request.reply_content else None,
            # image=request.image if request.image else None,
        )
        return ChatAgentResponse(
            status="success",
            response="success to call brain"
        )

    return router

def create_vector_store_router(usecase: DocumentLoader | None = None) -> APIRouter:
    router = APIRouter(prefix="/vector-store", tags=["vector-store"])
    loaders: dict[str, DocumentLoader] = {}

    if usecase is not None:
        loaders[usecase.collection_name] = usecase

    def get_agent_loader(agent: str | None) -> DocumentLoader:
        try:
            collection_name = DocumentLoader.resolve_collection_name(agent)
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err

        if collection_name not in loaders:
            loaders[collection_name] = DocumentLoader(collection_name=collection_name)

        return loaders[collection_name]

    @router.get("/supported-agent", response_model=list[str])
    def get_supported_agent() -> list[str]:
        return DocumentLoader.list_models()

    @router.get("/supported-document-type", response_model=SupportedDocumentType)
    def get_supported_document_type() -> SupportedDocumentType:
        return DocumentLoader.supported_document_type()

    @router.get("/documents", response_model=DocumentListResponse)
    def list_documents(agent: str | None = None) -> DocumentListResponse:
        return get_agent_loader(agent).get_list_saved_docs()

    @router.get("/documents/find", response_model=VectorStoreDocumentStatusResponse)
    def find_document(source: str, agent: str | None = None) -> VectorStoreDocumentStatusResponse:
        total_chunks = get_agent_loader(agent).count_document_by_source(source)
        return VectorStoreDocumentStatusResponse(
            source=source,
            exists=total_chunks > 0,
            total_chunks=total_chunks,
        )

    @router.get("/count-document-by-source", response_model=int)
    def count_document_by_source(source: str, agent: str | None = None) -> int:
        return get_agent_loader(agent).count_document_by_source(source)

    @router.delete("/documents", response_model=DocumentDeleteResult)
    def delete_document(source: str, agent: str | None = None) -> DocumentDeleteResult:
        return get_agent_loader(agent).delete_document_by_source(source)

    @router.delete("/delete-document-by-source", response_model=bool)
    def delete_document_by_source(source: str, agent: str | None = None) -> bool:
        return get_agent_loader(agent).delete_document_by_source(source).deleted
    
    @router.post("/ingest-document", response_model=DocumentIngestResult)
    def ingest_document(request: VectorStoreIngestRequest) -> DocumentIngestResult:
        try:
            agent_loader = get_agent_loader(request.agent)
            result = agent_loader.ingest_to_mongodb(
                folder_path=request.folder_path,
                urls=request.urls,
                file_paths=request.file_paths,
                category=request.category,
            )
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err
        except Exception as err:
            logger.exception("Failed to ingest document to vector store")
            raise HTTPException(status_code=500, detail=str(err)) from err

        if result is None:
            raise HTTPException(status_code=400, detail="Tidak ada dokumen yang dimuat")

        return result

    return router

def create_catalog_store_router(usecase: Catalog) -> APIRouter:
    router = APIRouter(prefix="/catalog", tags=["catalog"])

    @router.post(
        "",
        response_model=CatalogProduct,
        status_code=status.HTTP_201_CREATED,
        summary="Create catalog product",
        description=CATALOG_DETAILS_GUIDE,
        response_description="Created catalog product.",
    )
    def create_catalog_product(request: CatalogProduct) -> CatalogProduct:
        """Create a new catalog product."""
        try:
            return usecase.create_catalog(request)
        except ValueError as err:
            raise HTTPException(status_code=409, detail=str(err)) from err

    @router.get(
        "",
        response_model=list[CatalogProduct],
        summary="List catalog products",
        description="Return all catalog products with complete detail data.",
        response_description="All catalog products.",
    )
    def list_catalog_products() -> list[CatalogProduct]:
        """Read all catalog products."""
        return usecase.get_all_catalog()

    @router.get(
        "/preview",
        response_model=list[CatalogPreviewItem],
        summary="Preview catalog products",
        description="Return lightweight catalog data containing only id, product_name, description, and price.",
        response_description="Catalog preview items.",
    )
    def preview_catalog_products() -> list[CatalogPreviewItem]:
        """Read catalog preview data."""
        return usecase.get_preview_catalog()

    @router.get(
        "/by-ids",
        response_model=list[CatalogProduct],
        summary="Get catalog products by ids",
        description=(
            "Return catalog products matching the requested ids. "
            "Use repeated query params like `?ids=id1&ids=id2`, or comma-separated ids."
        ),
        response_description="Catalog products.",
    )
    def get_catalog_products_by_ids(
        ids: list[str] = Query(..., description="Catalog ids to retrieve."),
    ) -> list[CatalogProduct]:
        """Read multiple catalog products by ids."""
        catalog_ids: list[str] = []
        for raw_id in ids:
            for catalog_id in raw_id.split(","):
                clean_id = catalog_id.strip()
                if clean_id:
                    catalog_ids.append(clean_id)

        if not catalog_ids:
            raise HTTPException(status_code=400, detail="At least one catalog id is required")

        return usecase.get_catalog_by_id(catalog_ids)

    @router.get(
        "/{catalog_id}",
        response_model=CatalogProduct,
        summary="Get catalog product by id",
        description="Return one catalog product by its id.",
        response_description="Catalog product.",
    )
    def get_catalog_product(catalog_id: str) -> CatalogProduct:
        """Read one catalog product by id."""
        products = usecase.get_catalog_by_id([catalog_id])
        if not products:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return products[0]

    @router.get(
        "/{catalog_id}/images",
        response_model=list[ImageDetails],
        summary="List catalog product images",
        description="Return all images attached to one catalog product.",
        response_description="Catalog product images.",
    )
    def list_catalog_images(catalog_id: str) -> list[ImageDetails]:
        """Read all images for one catalog product."""
        images = usecase.get_catalog_images(catalog_id)
        if images is None:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return images

    @router.post(
        "/{catalog_id}/images/url",
        response_model=ImageDetails,
        status_code=status.HTTP_201_CREATED,
        summary="Attach catalog image URL",
        description="Attach an existing public image URL to one catalog product.",
        response_description="Attached catalog image.",
    )
    def add_catalog_image_url(catalog_id: str, request: CatalogImageURLRequest) -> ImageDetails:
        """Attach a public image URL to one catalog product."""
        try:
            image = usecase.add_catalog_image_url(catalog_id, request.url)
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err

        if image is None:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return image

    @router.post(
        "/{catalog_id}/images/upload",
        response_model=list[ImageDetails],
        status_code=status.HTTP_201_CREATED,
        summary="Upload catalog images",
        description="Upload one or more image files to Cloudflare R2 and attach the resulting URLs to the catalog product.",
        response_description="Uploaded catalog images.",
    )
    async def upload_catalog_images(
        catalog_id: str,
        files: list[UploadFile] = File(..., description="One or more image files."),
    ) -> list[ImageDetails]:
        """Upload image files for one catalog product."""
        if not files:
            raise HTTPException(status_code=400, detail="At least one image file is required")

        upload_files: list[tuple[str, bytes, str]] = []
        for upload_file in files:
            content_type = (upload_file.content_type or "").strip()
            if content_type and not content_type.lower().startswith("image/"):
                raise HTTPException(status_code=400, detail="Uploaded catalog files must be images")

            content = await upload_file.read()
            if not content:
                raise HTTPException(status_code=400, detail="Uploaded catalog image cannot be empty")

            upload_files.append((upload_file.filename or "catalog-image", content, content_type))

        try:
            uploaded_images = usecase.upload_catalog_images(catalog_id, upload_files)
        except ValueError as err:
            raise HTTPException(status_code=400, detail=str(err)) from err

        if uploaded_images is None:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return uploaded_images

    @router.delete(
        "/{catalog_id}/images/{image_id}",
        response_model=CatalogDeleteResponse,
        summary="Delete catalog image",
        description="Delete one image reference from the catalog product. R2-owned images are also deleted from Cloudflare R2.",
        response_description="Delete result.",
    )
    def delete_catalog_image(catalog_id: str, image_id: str) -> CatalogDeleteResponse:
        """Delete one image from one catalog product."""
        try:
            product = usecase.delete_catalog_image(catalog_id, image_id)
        except ValueError as err:
            raise HTTPException(status_code=404, detail=str(err)) from err

        if product is None:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return CatalogDeleteResponse(id=image_id, deleted=True)

    @router.put(
        "/{catalog_id}",
        response_model=CatalogProduct,
        summary="Update catalog product",
        description=CATALOG_DETAILS_GUIDE,
        response_description="Updated catalog product.",
    )
    def update_catalog_product(catalog_id: str, request: CatalogProduct) -> CatalogProduct:
        """Update an existing catalog product with a full replacement payload."""
        product = usecase.update_catalog(catalog_id, request)
        if product is None:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return product

    @router.delete(
        "/{catalog_id}",
        response_model=CatalogDeleteResponse,
        summary="Delete catalog product",
        description="Delete one catalog product by its id.",
        response_description="Delete result.",
    )
    def delete_catalog_product(catalog_id: str) -> CatalogDeleteResponse:
        """Delete one catalog product by id."""
        deleted = usecase.delete_catalog_by_id(catalog_id)
        if not deleted:
            raise HTTPException(status_code=404, detail="Catalog product not found")

        return CatalogDeleteResponse(id=catalog_id, deleted=True)

    return router

def create_admin_number_store_router(usecase: DBAcess) -> APIRouter:
    router = APIRouter(prefix="/admin/number-store", tags=["admin-number-store"])

    @router.get(
        "",
        response_model=AdminDataOutput,
        summary="List reply admins",
        description=REPLY_ADMIN_GUIDE,
        response_description="Reply admin database configuration.",
    )
    def list_reply_admins() -> AdminDataOutput:
        """Read reply admin configuration."""
        return usecase.get_admins_data()

    @router.get(
        "/{number}",
        response_model=AdminData,
        summary="Get reply admin by number",
        description="Return one reply admin by WhatsApp number.",
        response_description="Reply admin data.",
    )
    def get_reply_admin(number: str) -> AdminData:
        """Read one reply admin by number."""
        clean_number = number.strip()
        for admin in usecase.get_admins_data().numbers:
            if admin.number == clean_number:
                return admin

        raise HTTPException(status_code=404, detail="Reply admin not found")

    @router.post(
        "",
        response_model=AdminDataOutput,
        status_code=status.HTTP_201_CREATED,
        summary="Create reply admin",
        description=REPLY_ADMIN_GUIDE,
        response_description="Updated reply admin database configuration.",
    )
    def create_reply_admin(request: AdminData) -> AdminDataOutput:
        """Create a new reply admin."""
        clean_number = request.number.strip()
        if not clean_number:
            raise HTTPException(status_code=400, detail="number is required")

        current_admins = usecase.get_admins_data().numbers
        if any(admin.number == clean_number for admin in current_admins):
            raise HTTPException(status_code=409, detail="Reply admin already exists")

        saved = usecase.set_admins_data([request])
        if not saved:
            raise HTTPException(status_code=500, detail="Failed to create reply admin")

        return usecase.get_admins_data()

    @router.put(
        "/{number}",
        response_model=AdminDataOutput,
        summary="Update reply admin",
        description=REPLY_ADMIN_GUIDE,
        response_description="Updated reply admin database configuration.",
    )
    def update_reply_admin(number: str, request: AdminData) -> AdminDataOutput:
        """Update an existing reply admin."""
        original_number = number.strip()
        new_number = request.number.strip()
        if not original_number or not new_number:
            raise HTTPException(status_code=400, detail="number is required")

        current_admins = usecase.get_admins_data().numbers
        if not any(admin.number == original_number for admin in current_admins):
            raise HTTPException(status_code=404, detail="Reply admin not found")

        if new_number != original_number and any(admin.number == new_number for admin in current_admins):
            raise HTTPException(status_code=409, detail="Reply admin number already exists")

        if new_number != original_number:
            usecase.remove_admins_data([AdminData(name=request.name, number=original_number)])

        usecase.set_admins_data([request])

        return usecase.get_admins_data()

    @router.delete(
        "/{number}",
        response_model=AdminDeleteResponse,
        summary="Delete reply admin",
        description="Delete one reply admin by WhatsApp number.",
        response_description="Delete result.",
    )
    def delete_reply_admin(number: str) -> AdminDeleteResponse:
        """Delete one reply admin by number."""
        clean_number = number.strip()
        if not any(admin.number == clean_number for admin in usecase.get_admins_data().numbers):
            raise HTTPException(status_code=404, detail="Reply admin not found")

        deleted = usecase.remove_admins_data([AdminData(name=clean_number, number=clean_number)])
        if not deleted:
            raise HTTPException(status_code=500, detail="Failed to delete reply admin")

        return AdminDeleteResponse(number=clean_number, deleted=True)

    return router

class EscalationRequest(BaseModel):
    identifier: str = Field(description="WhatsApp JID or phone number identifier to check, enable or disable AI replies.")

class EscalationResponse(BaseModel):
    success: bool = Field(description="True if the operation succeeded.")

def clean_identifier(identifier: str) -> str:
    cleaned = (identifier or "").strip()
    for _ in range(3):
        if cleaned.startswith('"') and cleaned.endswith('"'):
            cleaned = cleaned[1:-1].strip()
        else:
            break
    return cleaned

def create_escalation_router(usecase: DBAcess) -> APIRouter:
    router = APIRouter(prefix="/escalation", tags=["escalation"])

    @router.get("/waiting", response_model=list[str])
    def get_waiting_list() -> list[str]:
        """Get the list of identifiers that are currently waiting for admin approval."""
        return usecase.get_waiting_for_admin_list()

    @router.get("/waiting/{identifier}", response_model=dict)
    def get_waiting_status(identifier: str) -> dict:
        """Get the identifier waiting status."""
        identifier = clean_identifier(identifier)
        waiting = usecase.get_waiting_for_admin(identifier)
        return {"identifier": identifier, "waiting": waiting is not None}

    @router.post("/disable", response_model=EscalationResponse)
    def disable_ai(request: EscalationRequest) -> EscalationResponse:
        """Add a phone number to the admin waiting list."""
        identifier = clean_identifier(request.identifier)
        success = usecase.disable_ai_reply(identifier)
        return EscalationResponse(success=success)

    @router.post("/enable", response_model=EscalationResponse)
    def enable_ai(request: EscalationRequest) -> EscalationResponse:
        """Remove a phone number from the admin waiting list."""
        identifier = clean_identifier(request.identifier)
        success = usecase.enable_ai_reply(identifier)
        return EscalationResponse(success=success)

    return router

class ScoringLogItem(BaseModel):
    timestamp: datetime | None = None
    question: str
    response: str
    ragas_faithfulness: float | None = None
    cosine_similarity: float | None = None
    aspects: dict[str, int | None] | None = None
    latency_ms: float | None = None
    response_length: float | None = None
    tool_call_count: float | None = None


class ScoringAverages(BaseModel):
    ragas_faithfulness: float | None = None
    cosine_similarity: float | None = None
    aspects: dict[str, float | None] | None = None
    latency_ms: float | None = None
    response_length: float | None = None
    tool_call_count: float | None = None


class ScoringReportResponse(BaseModel):
    agent_name: str
    count: int
    logs: list[ScoringLogItem]
    averages: ScoringAverages


# Canonical agent names stored on each scoring document.
SCORING_AGENTS = {"main", "reply", "research", "code"}


def _valid_scoring_agent(agent_name: str) -> bool:
    return agent_name == "all" or agent_name in SCORING_AGENTS


def create_scoring_agent(usecase: ScoringAgent) -> APIRouter:
    """GET /api/v1/scoring?agent_name=&count= -- score the last N logs of an agent."""
    router = APIRouter(prefix="/scoring", tags=["scoring"])

    @router.get("", response_model=ScoringReportResponse)
    async def show_agent_score(
        agent_name: str = Query(..., description="Agent to score: main, reply, research, or code."),
        count: int = Query(10, ge=1, le=100, description="Number of most recent logs to evaluate."),
    ) -> ScoringReportResponse:
        if agent_name not in SCORING_AGENTS:
            raise HTTPException(
                status_code=400,
                detail=f"Invalid agent_name '{agent_name}'. Expected one of: {sorted(SCORING_AGENTS)}.",
            )
        try:
            return await usecase.score(agent_name, count)
        except Exception:
            logger.exception("Scoring endpoint failed for agent=%s", agent_name)
            raise HTTPException(status_code=500, detail="scoring failed")

    @router.delete("")
    async def clear_agent_score(
        agent_name: str = Query(..., description="Agent to clear, or 'all'."),
    ):
        if not _valid_scoring_agent(agent_name):
            raise HTTPException(
                status_code=400,
                detail=f"Invalid agent_name '{agent_name}'. Expected one of: all, {sorted(SCORING_AGENTS)}.",
            )
        deleted = usecase.clear_scoring_data(agent_name)
        return {"agent_name": agent_name, "deleted": deleted}

    @router.get("/raw")
    async def raw_agent_score(
        agent_name: str = Query(..., description="Agent to export, or 'all'."),
        count: int = Query(50, ge=1, le=500, description="Max raw docs to return."),
    ):
        if not _valid_scoring_agent(agent_name):
            raise HTTPException(
                status_code=400,
                detail=f"Invalid agent_name '{agent_name}'. Expected one of: all, {sorted(SCORING_AGENTS)}.",
            )
        return usecase.get_raw_scoring_data(agent_name, count)

    return router