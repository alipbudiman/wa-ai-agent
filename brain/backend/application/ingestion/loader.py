from datetime import datetime, timezone
from glob import glob
import hashlib
import os
from pathlib import Path
import shutil
from typing import List

from langchain_core.documents import Document
from langchain_community.document_loaders import (
    CSVLoader,
    JSONLoader,
    PyPDFLoader,
    UnstructuredExcelLoader,
    UnstructuredPowerPointLoader,
    UnstructuredWordDocumentLoader,
    WebBaseLoader,
)
from langchain_mongodb import MongoDBAtlasVectorSearch
from langchain_openai import OpenAIEmbeddings
from langchain_text_splitters import RecursiveCharacterTextSplitter
from pydantic import BaseModel, Field

from backend.infrastructure.database.database import MongoDB
from config.config import Config


class DocumentInfo(BaseModel):
    source: str
    display_name: str = ""
    file_type: str = "unknown"
    category: str = "general"
    total_chunks: int = 0
    total_chars: int = 0
    document_key: str | None = None
    content_hash: str | None = None
    ingested_at: str | None = None


class DocumentListResponse(BaseModel):
    documents: List[DocumentInfo] = Field(default_factory=list)
    total_docs: int = 0

    @property
    def sources(self) -> List[str]:
        """Shortcut kalau hanya butuh list nama file."""
        return [d.source for d in self.documents]


class SupportedDocumentType(BaseModel):
    file_type: list[str]
    urls: list[str]


class IngestedDocumentInfo(BaseModel):
    source: str
    display_name: str = ""
    file_type: str = "unknown"
    category: str = "general"
    total_chunks: int = 0
    total_chars: int = 0
    document_key: str | None = None
    content_hash: str | None = None


class DocumentIngestResult(BaseModel):
    status: str = "success"
    message: str = "documents ingested to MongoDB vector store"
    total_sources: int = 0
    total_chunks: int = 0
    total_chars: int = 0
    replaced_chunks: int = 0
    sources: List[IngestedDocumentInfo] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)


class DocumentDeleteResult(BaseModel):
    source: str
    deleted: bool
    deleted_vectors: int = 0
    hard_deleted: bool = False
    hard_delete_error: str | None = None
    warning: str | None = None


class DocumentLoader:
    EMBEDDING_PROVIDER = "openai"
    EMBEDDING_MODEL = "text-embedding-3-small"
    EMBEDDING_DIMENSIONS = 1536
    VECTOR_INDEX_NAME = "vector_index"
    CHUNK_SIZE = 900
    CHUNK_OVERLAP = 120
    TEXT_ENCODINGS = ("utf-8-sig", "utf-8", "latin-1")

    def __init__(self):
        self.db = MongoDB()
        self.documents_collection = self.db.get_collection("documents")
        self.config = Config()
        self.open_ai_config = self.config.get_openai_config()
        self.embeddings = self.create_embeddings()
        self.supported_ext = ["*.pdf", "*.txt", "*.md", "*.csv", "*.docx", "*.xlsx", "*.pptx"]
        self.supported_urls = ["any web"]
        self.vector_store = MongoDBAtlasVectorSearch(
            collection=self.documents_collection,
            embedding=self.embeddings,
            index_name=self.VECTOR_INDEX_NAME,
        )
        self.create_vector_index(self.vector_store)

    def create_embeddings(self) -> OpenAIEmbeddings:
        self.open_ai_config = self.config.get_openai_config()
        return OpenAIEmbeddings(
            model=self.EMBEDDING_MODEL,
            api_key=self.open_ai_config.api_key,
        )

    def refresh_vector_store(self):
        """Refresh embedding client from the latest OpenAI config."""
        self.embeddings = self.create_embeddings()
        self.vector_store = MongoDBAtlasVectorSearch(
            collection=self.documents_collection,
            embedding=self.embeddings,
            index_name=self.VECTOR_INDEX_NAME,
        )
        return self.vector_store

    def get_retriever(self, k: int = 4, filter: dict = None):
        """Get retriever for RAG."""
        self.refresh_vector_store()
        search_kwargs = {"k": k}
        if filter:
            search_kwargs["filter"] = filter
        return self.vector_store.as_retriever(search_kwargs=search_kwargs)

    def similarity_search_with_relevance_scores(
        self,
        query: str,
        k: int = 4,
        score_threshold: float = 0.65,
        filter: dict = None,
    ) -> list[tuple[Document, float]]:
        """Search documents and keep only results above a relevance threshold."""
        self.refresh_vector_store()
        results = self.vector_store.similarity_search_with_relevance_scores(
            query,
            k=k,
            pre_filter=filter,
        )
        return [
            (doc, score)
            for doc, score in results
            if score is not None and score >= score_threshold
        ]

    def get_supported_document_type(self) -> SupportedDocumentType:
        return SupportedDocumentType(
            file_type=self.supported_ext,
            urls=self.supported_urls,
        )

    def _document_storage_root(self) -> Path:
        return Path(os.getenv("DOCUMENT_STORAGE_DIR", "uploads/documents")).expanduser().resolve()

    def _is_url(self, source: str) -> bool:
        return str(source or "").lower().startswith(("http://", "https://"))

    def _source_id(self, source: str) -> str:
        return hashlib.sha256(str(source or "").encode("utf-8")).hexdigest()

    def _document_key(self, source: str, content_hash: str | None = None) -> str:
        key_material = content_hash or str(source or "").strip()
        return hashlib.sha256(key_material.encode("utf-8")).hexdigest()

    def _display_name_from_source(self, source: str) -> str:
        source_text = str(source or "").strip()
        if not source_text:
            return ""
        if self._is_url(source_text):
            clean_source = source_text.rstrip("/")
            return clean_source.rsplit("/", 1)[-1] or clean_source
        return Path(source_text).name or source_text

    def _resolve_stored_path(self, raw_path: str, *, require_file: bool = False, require_dir: bool = False) -> Path:
        source_text = str(raw_path or "").strip()
        if not source_text:
            raise ValueError("Document path is required")

        source_path = Path(source_text).expanduser().resolve()
        storage_root = self._document_storage_root()

        try:
            source_path.relative_to(storage_root)
        except ValueError as exc:
            raise ValueError(f"Document path is outside storage root: {source_text}") from exc

        if require_file and not source_path.is_file():
            raise ValueError(f"Document file not found: {source_text}")

        if require_dir and not source_path.is_dir():
            raise ValueError(f"Document folder not found: {source_text}")

        return source_path

    def _resolve_deletable_document_path(self, source: str) -> Path | None:
        """Return source as a safe local document path, or None for URLs/unsupported paths."""
        source_text = str(source or "").strip()
        if not source_text or self._is_url(source_text):
            return None

        try:
            return self._resolve_stored_path(source_text)
        except ValueError:
            return None

    def _file_content_hash(self, file_path: Path) -> str:
        digest = hashlib.sha256()
        with file_path.open("rb") as file:
            for block in iter(lambda: file.read(1024 * 1024), b""):
                digest.update(block)
        return digest.hexdigest()

    def _load_text_document(self, file_path: Path) -> list[Document]:
        last_error: UnicodeDecodeError | None = None

        for encoding in self.TEXT_ENCODINGS:
            try:
                content = file_path.read_text(encoding=encoding)
                return [Document(page_content=content, metadata={"source": str(file_path)})]
            except UnicodeDecodeError as exc:
                last_error = exc

        raise RuntimeError(f"Error loading {file_path}: unsupported text encoding") from last_error

    def load_document(self, file_path: str) -> list[Document]:
        """Load document from a file path inside DOCUMENT_STORAGE_DIR."""
        source_path = self._resolve_stored_path(file_path, require_file=True)
        ext = source_path.suffix.lower()
        content_hash = self._file_content_hash(source_path)
        source = str(source_path)

        loaders = {
            ".pdf": lambda: PyPDFLoader(source).load(),
            ".txt": lambda: self._load_text_document(source_path),
            ".md": lambda: self._load_text_document(source_path),
            ".csv": lambda: CSVLoader(source).load(),
            ".docx": lambda: UnstructuredWordDocumentLoader(source).load(),
            ".xlsx": lambda: UnstructuredExcelLoader(source).load(),
            ".pptx": lambda: UnstructuredPowerPointLoader(source).load(),
            ".json": lambda: JSONLoader(source, jq_schema=".[]", text_content=False).load(),
        }

        loader_fn = loaders.get(ext)
        if not loader_fn:
            raise ValueError(f"Tipe file tidak didukung: {ext}")

        docs = loader_fn()
        base_metadata = {
            "file_type": ext.replace(".", ""),
            "source": source,
            "display_name": source_path.name,
            "stored_path": source,
            "content_hash": content_hash,
            "document_key": self._document_key(source, content_hash),
        }

        for doc in docs:
            doc.metadata.update(base_metadata)

        return docs

    def get_list_saved_docs(self) -> DocumentListResponse:
        """Get list of saved documents with detail info."""
        pipeline = [
            {"$match": {"source": {"$ne": None}}},
            {
                "$group": {
                    "_id": "$source",
                    "display_name": {"$first": "$display_name"},
                    "file_type": {"$first": "$file_type"},
                    "category": {"$first": "$category"},
                    "total_chunks": {"$sum": 1},
                    "total_chars": {"$sum": "$char_count"},
                    "document_key": {"$first": "$document_key"},
                    "content_hash": {"$first": "$content_hash"},
                    "ingested_at": {"$max": "$ingested_at"},
                }
            },
            {"$sort": {"display_name": 1, "_id": 1}},
        ]

        documents = []
        for row in self.documents_collection.aggregate(pipeline):
            source = row["_id"]
            documents.append(
                DocumentInfo(
                    source=source,
                    display_name=row.get("display_name") or self._display_name_from_source(source),
                    file_type=row.get("file_type", "unknown"),
                    category=row.get("category", "general"),
                    total_chunks=row.get("total_chunks", 0),
                    total_chars=row.get("total_chars", 0),
                    document_key=row.get("document_key"),
                    content_hash=row.get("content_hash"),
                    ingested_at=row.get("ingested_at"),
                )
            )

        return DocumentListResponse(
            documents=documents,
            total_docs=len(documents),
        )

    def count_document_by_source(self, source: str) -> int:
        """Find document by source."""
        return self.documents_collection.count_documents({"source": source})

    def delete_document_vectors_by_source(self, source: str) -> int:
        """Delete vector chunks for a source without deleting the hard document."""
        res = self.documents_collection.delete_many({"source": source})
        return res.deleted_count

    def delete_stale_document_vectors_by_source(self, source: str, keep_batch_id: str) -> int:
        """Delete old vector chunks for a source after a replacement batch succeeds."""
        res = self.documents_collection.delete_many({
            "source": source,
            "ingest_batch_id": {"$ne": keep_batch_id},
        })
        return res.deleted_count

    def delete_stale_document_vectors_by_document_key(self, document_key: str, keep_batch_id: str) -> int:
        """Delete old vector chunks for a stable document key after a replacement succeeds."""
        res = self.documents_collection.delete_many({
            "document_key": document_key,
            "ingest_batch_id": {"$ne": keep_batch_id},
        })
        return res.deleted_count

    def _existing_sources_by_document_key(self, document_keys: set[str]) -> dict[str, set[str]]:
        if not document_keys:
            return {}

        pipeline = [
            {"$match": {"document_key": {"$in": list(document_keys)}}},
            {"$group": {"_id": "$document_key", "sources": {"$addToSet": "$source"}}},
        ]

        existing: dict[str, set[str]] = {}
        for row in self.documents_collection.aggregate(pipeline):
            existing[row["_id"]] = {source for source in row.get("sources", []) if source}
        return existing

    def _delete_hard_document_by_source(self, source: str) -> tuple[bool, str | None]:
        source_path = self._resolve_deletable_document_path(source)
        if source_path is None or not source_path.exists():
            return False, None

        try:
            if source_path.is_dir():
                shutil.rmtree(source_path)
            else:
                source_path.unlink()

                parent = source_path.parent
                storage_root = self._document_storage_root()
                try:
                    parent.relative_to(storage_root)
                    if parent != storage_root and not any(parent.iterdir()):
                        parent.rmdir()
                except (OSError, ValueError):
                    pass
        except OSError as exc:
            return False, str(exc)

        return True, None

    def delete_document_by_source(self, source: str) -> DocumentDeleteResult:
        """Delete document vectors and the stored hard document."""
        res = self.documents_collection.delete_many({"source": source})
        hard_deleted, hard_delete_error = self._delete_hard_document_by_source(source)
        warning = None

        if hard_delete_error:
            warning = f"Vector chunks deleted, but hard document cleanup failed: {hard_delete_error}"

        return DocumentDeleteResult(
            source=source,
            deleted=res.deleted_count > 0 or hard_deleted,
            deleted_vectors=res.deleted_count,
            hard_deleted=hard_deleted,
            hard_delete_error=hard_delete_error,
            warning=warning,
        )

    def load_directory(self, folder_path: str) -> list[Document]:
        """Load semua dokumen dalam satu folder."""
        directory = self._resolve_stored_path(folder_path, require_dir=True)
        all_docs = []
        failed = []

        for pattern in self.supported_ext:
            files = glob(str(directory / "**" / pattern), recursive=True)
            for file in files:
                try:
                    docs = self.load_document(file)
                    all_docs.extend(docs)
                    print(f"Loaded {os.path.basename(file)} -> {len(docs)} pages")
                except Exception as exc:
                    failed.append(file)
                    print(f"Failed loading {os.path.basename(file)}: {exc}")

        print(f"Total loaded documents: {len(all_docs)} | failed files: {len(failed)}")
        return all_docs

    def load_urls(self, urls: list[str]) -> list[Document]:
        """Load dari banyak URL sekaligus."""
        clean_urls = [url.strip() for url in urls if str(url or "").strip()]
        loader = WebBaseLoader(clean_urls)
        docs = loader.load()

        for doc in docs:
            source = str(doc.metadata.get("source") or doc.metadata.get("url") or "").strip()
            if not source and len(clean_urls) == 1:
                source = clean_urls[0]

            doc.metadata.update({
                "file_type": "web",
                "source": source,
                "display_name": self._display_name_from_source(source),
                "stored_path": None,
                "content_hash": None,
                "document_key": self._document_key(source),
            })

        return docs

    def ingest_to_mongodb(
        self,
        folder_path: str = None,
        urls: list = None,
        file_paths: list = None,
        category: str = "general",
    ) -> DocumentIngestResult | None:
        """Ingest documents into MongoDB vector store."""
        self.refresh_vector_store()
        if self.open_ai_config.api_key == "":
            raise ValueError("OpenAI API key is required. Please set it in the configuration. if you in bot, type `/openai-config` to set it.")

        clean_category = (category or "").strip() or "general"
        all_docs = []

        if folder_path:
            all_docs += self.load_directory(folder_path)

        if urls:
            all_docs += self.load_urls(urls)

        if file_paths:
            for file_path in file_paths:
                all_docs += self.load_document(file_path)

        if not all_docs:
            print("Tidak ada dokumen yang dimuat")
            return None

        splitter = RecursiveCharacterTextSplitter(
            chunk_size=self.CHUNK_SIZE,
            chunk_overlap=self.CHUNK_OVERLAP,
        )
        chunks = splitter.split_documents(all_docs)
        if not chunks:
            print("Tidak ada chunk yang dibuat")
            return None

        source_chunk_counters: dict[str, int] = {}
        source_stats: dict[str, dict] = {}
        document_keys: set[str] = set()
        sources_by_document_key: dict[str, set[str]] = {}
        ingested_at = datetime.now(timezone.utc).isoformat()
        ingest_batch_id = self._source_id(f"{ingested_at}:{clean_category}")

        for chunk_index, chunk in enumerate(chunks):
            source = str(chunk.metadata.get("source", "")).strip()
            if not source:
                source = f"unknown:{ingest_batch_id}:{chunk_index}"

            content_hash = chunk.metadata.get("content_hash")
            document_key = chunk.metadata.get("document_key") or self._document_key(source, content_hash)
            display_name = chunk.metadata.get("display_name") or self._display_name_from_source(source)
            file_type = chunk.metadata.get("file_type", "unknown")

            source_chunk_index = source_chunk_counters.get(source, 0)
            source_chunk_counters[source] = source_chunk_index + 1
            document_keys.add(document_key)
            sources_by_document_key.setdefault(document_key, set()).add(source)

            chunk.metadata.update({
                "chunk_id": chunk_index,
                "chunk_index": source_chunk_index,
                "document_id": document_key,
                "document_key": document_key,
                "display_name": display_name,
                "source": source,
                "file_type": file_type,
                "category": clean_category,
                "char_count": len(chunk.page_content),
                "ingested_at": ingested_at,
                "ingest_batch_id": ingest_batch_id,
                "embedding_provider": self.EMBEDDING_PROVIDER,
                "embedding_model": self.EMBEDDING_MODEL,
                "embedding_dimensions": self.EMBEDDING_DIMENSIONS,
            })

            stats = source_stats.setdefault(
                source,
                {
                    "source": source,
                    "display_name": display_name,
                    "file_type": file_type,
                    "category": clean_category,
                    "total_chunks": 0,
                    "total_chars": 0,
                    "document_key": document_key,
                    "content_hash": content_hash,
                },
            )
            stats["total_chunks"] += 1
            stats["total_chars"] += len(chunk.page_content)

        existing_sources_by_key = self._existing_sources_by_document_key(document_keys)

        self.vector_store.add_documents(chunks)

        replaced_chunks = 0
        for source in source_chunk_counters:
            replaced_chunks += self.delete_stale_document_vectors_by_source(source, ingest_batch_id)

        for document_key in document_keys:
            replaced_chunks += self.delete_stale_document_vectors_by_document_key(document_key, ingest_batch_id)

        warnings = []
        for document_key, old_sources in existing_sources_by_key.items():
            current_sources = sources_by_document_key.get(document_key, set())
            for old_source in old_sources - current_sources:
                _, cleanup_error = self._delete_hard_document_by_source(old_source)
                if cleanup_error:
                    warnings.append(f"Failed to cleanup replaced document {old_source}: {cleanup_error}")

        sources = [
            IngestedDocumentInfo(**stats)
            for stats in sorted(source_stats.values(), key=lambda item: (item["display_name"], item["source"]))
        ]

        print(f"Berhasil simpan {len(chunks)} chunks ke MongoDB")
        return DocumentIngestResult(
            total_sources=len(sources),
            total_chunks=len(chunks),
            total_chars=sum(item.total_chars for item in sources),
            replaced_chunks=replaced_chunks,
            sources=sources,
            warnings=warnings,
        )

    def create_vector_index(self, v_store):
        """
        Create vector search index.

        Args:
            v_store: Vector store
        """
        index_name = self.VECTOR_INDEX_NAME

        try:
            existing_indexes = list(self.documents_collection.list_search_indexes())
            index_exists = any(idx.get("name") == index_name for idx in existing_indexes)

            if index_exists:
                print(f"Index '{index_name}' sudah ada, melewati proses pembuatan.")
                return

            print(f"Membuat vector search index '{index_name}'...")
            v_store.create_vector_search_index(
                dimensions=self.EMBEDDING_DIMENSIONS,
                wait_until_complete=30,
            )
            print(f"Index '{index_name}' berhasil dibuat.")

        except Exception as exc:
            print(f"Gagal mengecek/membuat index: {exc}")
            v_store.create_vector_search_index(
                dimensions=self.EMBEDDING_DIMENSIONS,
                wait_until_complete=30,
            )
