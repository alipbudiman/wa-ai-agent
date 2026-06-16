from __future__ import annotations

import hashlib
import hmac
import logging
import xml.etree.ElementTree as ET
from collections.abc import Iterable
from datetime import datetime, timezone
from urllib.parse import parse_qsl, quote, unquote, urlencode, urlsplit

import requests
from pydantic import BaseModel

from config.config import CloudflareR2Config, Config


logger = logging.getLogger("uvicorn.error")


class CloudflareR2UploadResponse(BaseModel):
    key: str = ""
    url: str = ""
    bucket: str = ""
    version_id: str = ""
    etag: str = ""
    size: int = 0
    error: str = ""


class CloudflareR2Storage:
    def __init__(self, config: Config | None = None, r2_config: CloudflareR2Config | None = None):
        self.config = config
        self.r2_config = r2_config

    def upload_image(
        self,
        image: bytes,
        file_name: str,
        content_type: str = "",
        directory_name: str = "",
    ) -> CloudflareR2UploadResponse:
        if not image:
            raise ValueError("image data cannot be empty")
        if not file_name:
            raise ValueError("file_name cannot be empty")
        if not content_type:
            content_type = "image/jpeg"

        return self._upload_file(
            file_data=image,
            file_name=file_name,
            content_type=content_type,
            directory_name=directory_name,
            timeout=60,
        )

    def upload_audio(
        self,
        audio: bytes,
        file_name: str,
        content_type: str = "",
        directory_name: str = "",
    ) -> CloudflareR2UploadResponse:
        if not audio:
            raise ValueError("audio data cannot be empty")
        if not file_name:
            raise ValueError("file_name cannot be empty")
        if not directory_name:
            raise ValueError("directory_name cannot be empty")
        if not content_type:
            content_type = "audio/mpeg"

        return self._upload_file(
            file_data=audio,
            file_name=file_name,
            content_type=content_type,
            directory_name=directory_name,
            timeout=120,
        )

    def check_object_exists(self, object_key: str, directory_name: str = "") -> bool:
        if not object_key:
            raise ValueError("object_key cannot be empty")

        full_object_key = self._build_object_key(directory_name, object_key)
        response = self._send_r2_request(
            method="HEAD",
            object_key=full_object_key,
            payload=b"",
            timeout=15,
        )

        if response.status_code == requests.codes.ok:
            return True
        if response.status_code == requests.codes.not_found:
            return False

        self._raise_for_status(response, {requests.codes.ok})
        return False

    def delete_object(self, object_key: str, directory_name: str = "") -> None:
        if not object_key:
            raise ValueError("object_key cannot be empty")

        full_object_key = self._build_object_key(directory_name, object_key)
        response = self._send_r2_request(
            method="DELETE",
            object_key=full_object_key,
            payload=b"",
            timeout=30,
        )
        self._raise_for_status(response, {requests.codes.no_content, requests.codes.ok})

    def get_public_object_key(self, public_url: str) -> str | None:
        """Return the R2 object key for URLs served from this bucket public URL."""
        raw_url = (public_url or "").strip()
        if not raw_url:
            return None

        if self.r2_config is not None:
            r2_config = self.r2_config
        else:
            config = self.config or Config()
            r2_config = config.get_cloudflare_r2_config()

        base_url = (r2_config.public_url or "").strip().rstrip("/")
        if not base_url:
            return None
        if not base_url.startswith(("http://", "https://")):
            base_url = f"https://{base_url}"

        parsed_base = urlsplit(base_url)
        parsed_url = urlsplit(raw_url)
        if parsed_url.scheme != parsed_base.scheme or parsed_url.netloc != parsed_base.netloc:
            return None

        base_path = parsed_base.path.rstrip("/")
        if base_path and not (
            parsed_url.path == base_path or parsed_url.path.startswith(f"{base_path}/")
        ):
            return None

        object_key = parsed_url.path[len(base_path):].lstrip("/")
        if not object_key:
            return None

        return unquote(object_key)

    def create_directory(self, directory_name: str) -> None:
        dir_key = directory_name.strip("/")
        if not dir_key:
            raise ValueError("directory_name cannot be empty")

        response = self._send_r2_request(
            method="PUT",
            object_key=f"{dir_key}/",
            payload=b"",
            content_type="application/x-directory",
            include_content_length=True,
            timeout=30,
        )
        self._raise_for_status(response, {requests.codes.ok, requests.codes.created, requests.codes.no_content})

    def check_directory(self, directory_name: str) -> bool:
        dir_key = directory_name.strip("/")
        if not dir_key:
            raise ValueError("directory_name cannot be empty")

        response = self._send_r2_request(
            method="GET",
            object_key="",
            payload=b"",
            query_params={
                "list-type": "2",
                "prefix": f"{dir_key}/",
                "max-keys": "1",
            },
            timeout=15,
        )
        self._raise_for_status(response, {requests.codes.ok})

        parsed = self._parse_list_bucket_result(response.content)
        return parsed["key_count"] > 0 or bool(parsed["contents"])

    def delete_directory(self, directory_name: str) -> None:
        dir_key = directory_name.strip("/")
        if not dir_key:
            raise ValueError("directory_name cannot be empty")

        prefix = f"{dir_key}/"
        object_keys = self._list_objects_with_prefix(prefix)
        if not object_keys:
            return

        delete_errors: list[str] = []
        for key in object_keys:
            if not key:
                continue
            try:
                self.delete_object(key)
            except Exception as exc:
                delete_errors.append(f"{key}: {exc}")

        if delete_errors:
            raise RuntimeError(f"failed to delete some objects under {prefix}: {'; '.join(delete_errors)}")

    def get_list_directory(self, *except_dirs: str) -> list[str]:
        excluded = {
            item.strip().strip("/")
            for item in except_dirs
            if item and item.strip().strip("/")
        }
        directories: set[str] = set()
        continuation_token = ""

        while True:
            params = {
                "list-type": "2",
                "delimiter": "/",
            }
            if continuation_token:
                params["continuation-token"] = continuation_token

            response = self._send_r2_request(
                method="GET",
                object_key="",
                payload=b"",
                query_params=params,
                timeout=30,
            )
            self._raise_for_status(response, {requests.codes.ok})

            parsed = self._parse_list_bucket_result(response.content)
            for prefix in parsed["common_prefixes"]:
                dir_name = prefix.strip("/")
                if dir_name and dir_name not in excluded:
                    directories.add(dir_name)

            for key in parsed["contents"]:
                normalized_key = key.strip("/")
                if not normalized_key or "/" in normalized_key:
                    continue
                if not key.endswith("/"):
                    continue
                if normalized_key not in excluded:
                    directories.add(normalized_key)

            if not parsed["is_truncated"] or not parsed["next_continuation_token"]:
                break
            continuation_token = parsed["next_continuation_token"]

        return sorted(directories)

    def _upload_file(
        self,
        file_data: bytes,
        file_name: str,
        content_type: str,
        directory_name: str,
        timeout: int,
    ) -> CloudflareR2UploadResponse:
        r2_config = self._get_r2_config()
        object_key = self._build_object_key(directory_name, file_name)
        response = self._send_r2_request(
            method="PUT",
            object_key=object_key,
            payload=file_data,
            content_type=content_type,
            include_content_length=True,
            timeout=timeout,
        )
        self._raise_for_status(response, {requests.codes.ok, requests.codes.created})

        return CloudflareR2UploadResponse(
            key=object_key,
            url=self._build_public_url(r2_config.public_url, object_key),
            bucket=r2_config.bucket_name,
            version_id=response.headers.get("x-amz-version-id", ""),
            etag=response.headers.get("ETag", "").strip('"'),
            size=len(file_data),
            error="",
        )

    def _list_objects_with_prefix(self, prefix: str) -> list[str]:
        if not prefix:
            raise ValueError("prefix cannot be empty")

        object_keys: list[str] = []
        continuation_token = ""

        while True:
            params = {
                "list-type": "2",
                "prefix": prefix,
            }
            if continuation_token:
                params["continuation-token"] = continuation_token

            response = self._send_r2_request(
                method="GET",
                object_key="",
                payload=b"",
                query_params=params,
                timeout=30,
            )
            self._raise_for_status(response, {requests.codes.ok})

            parsed = self._parse_list_bucket_result(response.content)
            object_keys.extend(key for key in parsed["contents"] if key)

            if not parsed["is_truncated"] or not parsed["next_continuation_token"]:
                break
            continuation_token = parsed["next_continuation_token"]

        return object_keys

    def _send_r2_request(
        self,
        method: str,
        object_key: str,
        payload: bytes = b"",
        content_type: str | None = None,
        include_content_length: bool = False,
        query_params: dict[str, str] | None = None,
        timeout: int = 30,
    ) -> requests.Response:
        r2_config = self._get_r2_config()
        encoded_key = self._encode_object_key(object_key) if object_key else ""
        request_url = self._build_r2_url(r2_config, encoded_key, query_params)
        headers: dict[str, str] = {}

        if content_type is not None:
            headers["Content-Type"] = content_type
        if include_content_length:
            headers["Content-Length"] = str(len(payload))

        self._sign_request(method, request_url, headers, payload, encoded_key, r2_config)

        try:
            return requests.request(
                method=method,
                url=request_url,
                data=payload if method.upper() == "PUT" else None,
                headers=headers,
                timeout=timeout,
            )
        except requests.RequestException as exc:
            logger.error("Failed to execute R2 request: %s", exc)
            raise RuntimeError(f"failed to execute R2 request: {exc}") from exc

    def _sign_request(
        self,
        method: str,
        request_url: str,
        headers: dict[str, str],
        payload: bytes,
        encoded_object_key: str,
        r2_config: CloudflareR2Config,
    ) -> None:
        now = datetime.now(timezone.utc)
        date_stamp = now.strftime("%Y%m%d")
        amz_date = now.strftime("%Y%m%dT%H%M%SZ")

        headers["x-amz-date"] = amz_date
        headers["x-amz-content-sha256"] = self._hash_sha256(payload)

        region = "auto"
        service = "s3"
        credential_scope = f"{date_stamp}/{region}/{service}/aws4_request"
        canonical_request = self._create_canonical_request(
            method=method,
            request_url=request_url,
            headers=headers,
            object_key=encoded_object_key,
            bucket=r2_config.bucket_name,
        )
        string_to_sign = "\n".join(
            [
                "AWS4-HMAC-SHA256",
                amz_date,
                credential_scope,
                self._hash_sha256(canonical_request.encode()),
            ]
        )
        signature = self._calculate_signature(
            secret_key=r2_config.secret_access_key,
            date_stamp=date_stamp,
            region=region,
            service=service,
            string_to_sign=string_to_sign,
        )
        signed_headers = self._build_signed_headers(headers, urlsplit(request_url).netloc)
        headers["Authorization"] = (
            "AWS4-HMAC-SHA256 "
            f"Credential={r2_config.access_key_id}/{credential_scope}, "
            f"SignedHeaders={signed_headers}, "
            f"Signature={signature}"
        )

    def _create_canonical_request(
        self,
        method: str,
        request_url: str,
        headers: dict[str, str],
        object_key: str,
        bucket: str,
    ) -> str:
        parsed_url = urlsplit(request_url)
        canonical_uri = f"/{bucket}" if not object_key else f"/{bucket}/{object_key}"

        return "\n".join(
            [
                method.upper(),
                canonical_uri,
                self._build_canonical_query_string(request_url),
                self._build_canonical_headers(headers, parsed_url.netloc),
                self._build_signed_headers(headers, parsed_url.netloc),
                headers.get("x-amz-content-sha256", ""),
            ]
        )

    def _build_canonical_headers(self, headers: dict[str, str], host: str) -> str:
        canonical_headers: list[str] = []
        has_host = False

        for key, value in headers.items():
            lower_key = key.lower()
            if not self._is_signed_header(lower_key):
                continue
            if lower_key == "host":
                has_host = True
            canonical_headers.append(f"{lower_key}:{str(value).strip()}")

        if not has_host and host:
            canonical_headers.append(f"host:{host}")

        canonical_headers.sort()
        return "\n".join(canonical_headers) + "\n"

    def _build_signed_headers(self, headers: dict[str, str], host: str) -> str:
        signed_headers = {
            key.lower()
            for key in headers
            if self._is_signed_header(key.lower())
        }

        if host:
            signed_headers.add("host")

        return ";".join(sorted(signed_headers))

    def _get_r2_config(self) -> CloudflareR2Config:
        if self.r2_config is not None:
            r2_config = self.r2_config
        else:
            config = self.config or Config()
            r2_config = config.get_cloudflare_r2_config()
        self._validate_r2_config(r2_config)
        return r2_config

    def _validate_r2_config(self, r2_config: CloudflareR2Config) -> None:
        if not r2_config.account_id:
            raise ValueError("R2_ACCOUNT_ID not configured")
        if not r2_config.access_key_id:
            raise ValueError("R2_ACCESS_KEY_ID not configured")
        if not r2_config.secret_access_key:
            raise ValueError("R2_SECRET_ACCESS_KEY not configured")
        if not r2_config.bucket_name:
            raise ValueError("R2_BUCKET_NAME not configured")

    def _build_r2_url(
        self,
        r2_config: CloudflareR2Config,
        encoded_key: str,
        query_params: dict[str, str] | None = None,
    ) -> str:
        endpoint = f"https://{r2_config.account_id}.r2.cloudflarestorage.com"
        request_url = f"{endpoint}/{r2_config.bucket_name}"
        if encoded_key:
            request_url = f"{request_url}/{encoded_key}"
        if query_params:
            request_url = f"{request_url}?{urlencode(query_params)}"
        return request_url

    def _build_public_url(self, public_url: str, object_key: str) -> str:
        base_url = public_url.rstrip("/")
        if base_url.startswith(("http://", "https://")):
            return f"{base_url}/{object_key}"
        return f"https://{base_url}/{object_key}"

    def _build_object_key(self, directory_name: str, file_name: str) -> str:
        directory = directory_name.strip("/")
        name = file_name.lstrip("/")
        if not directory:
            return name
        return f"{directory}/{name}"

    def _encode_object_key(self, object_key: str) -> str:
        return "/".join(quote(part, safe="") for part in object_key.split("/"))

    def _build_canonical_query_string(self, request_url: str) -> str:
        parsed_url = urlsplit(request_url)
        query_pairs = parse_qsl(parsed_url.query, keep_blank_values=True)
        if not query_pairs:
            return ""

        query_by_key: dict[str, list[str]] = {}
        for key, value in query_pairs:
            query_by_key.setdefault(key, []).append(value)

        parts: list[str] = []
        for key in sorted(query_by_key):
            for value in sorted(query_by_key[key]):
                parts.append(f"{self._aws_url_encode(key)}={self._aws_url_encode(value)}")

        return "&".join(parts)

    def _parse_list_bucket_result(self, xml_bytes: bytes) -> dict[str, object]:
        root = ET.fromstring(xml_bytes)
        return {
            "key_count": self._int_or_zero(self._find_text(root, "KeyCount")),
            "contents": [
                self._find_text(content, "Key")
                for content in self._find_children(root, "Contents")
                if self._find_text(content, "Key")
            ],
            "common_prefixes": [
                self._find_text(prefix, "Prefix")
                for prefix in self._find_children(root, "CommonPrefixes")
                if self._find_text(prefix, "Prefix")
            ],
            "is_truncated": self._find_text(root, "IsTruncated").lower() == "true",
            "next_continuation_token": self._find_text(root, "NextContinuationToken"),
        }

    def _raise_for_status(self, response: requests.Response, success_statuses: Iterable[int]) -> None:
        if response.status_code in success_statuses:
            return
        raise RuntimeError(f"R2 API returned status {response.status_code}: {response.text}")

    def _calculate_signature(
        self,
        secret_key: str,
        date_stamp: str,
        region: str,
        service: str,
        string_to_sign: str,
    ) -> str:
        k_date = self._hmac_sha256(f"AWS4{secret_key}".encode(), date_stamp.encode())
        k_region = self._hmac_sha256(k_date, region.encode())
        k_service = self._hmac_sha256(k_region, service.encode())
        k_signing = self._hmac_sha256(k_service, b"aws4_request")
        signature = self._hmac_sha256(k_signing, string_to_sign.encode())
        return signature.hex()

    def _hmac_sha256(self, key: bytes, data: bytes) -> bytes:
        return hmac.new(key, data, hashlib.sha256).digest()

    def _hash_sha256(self, data: bytes) -> str:
        return hashlib.sha256(data).hexdigest()

    def _aws_url_encode(self, value: str) -> str:
        return quote(value, safe="-_.~")

    def _is_signed_header(self, lower_key: str) -> bool:
        return (
            lower_key == "host"
            or lower_key.startswith("x-amz-")
            or lower_key == "content-type"
            or lower_key == "content-length"
        )

    def _find_children(self, element: ET.Element, local_name: str) -> list[ET.Element]:
        return [
            child
            for child in element
            if self._local_name(child.tag) == local_name
        ]

    def _find_text(self, element: ET.Element, local_name: str) -> str:
        for child in element:
            if self._local_name(child.tag) == local_name:
                return (child.text or "").strip()
        return ""

    def _local_name(self, tag: str) -> str:
        return tag.rsplit("}", 1)[-1]

    def _int_or_zero(self, value: str) -> int:
        try:
            return int(value)
        except ValueError:
            return 0
