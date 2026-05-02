

from config.config import Config
from typing import TypedDict
from typing import Any
import uuid
import base64
from backend.application.ai.tools.models import CommandInputGenerateImage
from langchain_core.tools import StructuredTool
from backend.application.ai.tools.whatsapp_integration import WhatsAppIntegration
import shutil
import os
from pathlib import Path
import requests
import logging
import time
import random

from google import genai
from google.genai import types
from PIL import Image
from io import BytesIO

logger = logging.getLogger("uvicorn.error")

class PromptsType(TypedDict):
    prompts: list[str]

class FileX: 
    def __init__(self):
        self._playground_root = self.__resolve_folder_root("playground")
        self._run_subagent_root = self.__resolve_folder_root("playground/runSubagent")
        self._agent_skills_root = self.__resolve_folder_root(".agents")
        self._claude_instructions_root = self.__resolve_folder_root(".claude")
        self.wa_tools = WhatsAppIntegration()
        self.config = Config()
        self.google_config = self.config.get_google_config()
        self.minimax_config = self.config.get_minimax_config()
        self.gemini_image_model = "imagen-4.0-generate-001"
        self.minimax_image_model = "image-01"

    def __resolve_folder_root(self, folder_name: str = "playground") -> Path:
        """Resolve repository playground root for file guard checks."""
        current_file = Path(__file__).resolve()

        # Prefer existing playground discovered from cwd first for runtime flexibility.
        cwd_candidate = (Path.cwd().resolve() / folder_name).resolve()
        if cwd_candidate.exists():
            return cwd_candidate

        for parent in current_file.parents:
            candidate = (parent / folder_name).resolve()
            if candidate.exists():
                return candidate

        # Fallback path when playground is not created yet.
        print(f"Warning: '{folder_name}' folder not found in any parent directories. Using fallback path: {cwd_candidate}")
        return cwd_candidate

    def __normalize_delete_target(self, file_path: str) -> Path:
        """Normalize target path with safe defaults toward playground."""
        raw = (file_path or "").strip().strip('`"\'')
        if not raw:
            raise ValueError("file_path is required")

        target = Path(raw).expanduser()
        if target.is_absolute():
            return target.resolve(strict=False)

        normalized = raw.replace("\\", "/")
        if normalized == "playground" or normalized.startswith("playground/"):
            return (self._playground_root.parent / normalized).resolve(strict=False)

        return (self._playground_root / target).resolve(strict=False)

    def __is_inside_playground(self, target_path: Path) -> bool:
        try:
            target_path.relative_to(self._playground_root)
            return True
        except ValueError:
            return False

    def compress(self, file_path: str, format: str = "zip", base_name: str = "output_archive") -> str:
        """
        Compress a file or folder to a 'zip', 'tar', 'gztar', 'bztar', 'xztar' file.
        
        Args:
            base_name (str): The name of the compressed file (without extension).
            file_path (str): The path to the file or folder to compress.
            format (str): The format of the compressed file.
        
        Returns:
            str: The path to the compressed file.
        """
        file_path = os.path.abspath(file_path)
        
        # Don't add double extensions if AI provided one
        if base_name.endswith(f".{format}"):
            base_name = base_name[:-(len(format)+1)]

        # If base_name is not absolute, save the archive in the target's parent directory
        if not os.path.isabs(base_name):
            parent_dir = os.path.dirname(file_path)
            base_name = os.path.join(parent_dir, base_name)

        if os.path.isfile(file_path):
            root_dir = os.path.dirname(file_path)
            base_dir = os.path.basename(file_path)
            res = shutil.make_archive(
                base_name=base_name,
                format=format,
                root_dir=root_dir,
                base_dir=base_dir
            )
        elif os.path.isdir(file_path):
            res = shutil.make_archive(
                base_name=base_name,
                format=format,
                root_dir=file_path
            )
        else:
            list_file_in_dir = [file for file in os.listdir(os.path.dirname(file_path))] if os.path.isdir(os.path.dirname(file_path)) else []
            return f"File or directory not found: {file_path}\nList of files in directory: {list_file_in_dir}"
            
        return res
    
    def delete_file_dir(self, file_path: str) -> str:
        """
        Delete file and directory
        
        Args:
            file_path (str): The path to the file or directory to delete.
        
        Returns:
            str: A message indicating the file or directory has been deleted.
        """

        target_path = self.__normalize_delete_target(file_path)

        if target_path == self._playground_root:
            return "Deletion blocked: playground root directory cannot be deleted."

        if not self.__is_inside_playground(target_path):
            return f"Deletion blocked: only files/folders inside '{self._playground_root}' can be deleted."

        if not target_path.exists() and not target_path.is_symlink():
            return f"Path not found: {target_path}"

        if target_path.is_symlink() or target_path.is_file():
            target_path.unlink(missing_ok=True)
            return f"File deleted: {target_path}"

        if target_path.is_dir():
            shutil.rmtree(target_path)
            return f"Directory deleted: {target_path}"

        return f"Deletion skipped: unsupported path type for {target_path}"
    
    def __gen_image_by_gemini(self, prompt: str, output_path: str):
        """Generate gambar dari teks prompt."""
        google_config = self.config.get_google_config()
        if google_config.api_key == "":
            raise Exception("Google API Key not found. Please set it using /google-config.")
        client = genai.Client(api_key=google_config.api_key)
        response = client.models.generate_content(
            model=self.gemini_image_model,
            contents=prompt,
            config=types.GenerateContentConfig(
                response_modalities=["TEXT", "IMAGE"]
            )
        )
    
        for part in response.candidates[0].content.parts:
            if part.inline_data is not None:
                image = Image.open(BytesIO(part.inline_data.data))
                image.save(output_path)
                print(f"✅ Gambar disimpan: {output_path}")
                return output_path
            elif part.text:
                print(f"💬 Model: {part.text}")
    
        return ""
    
    def __gen_image_by_minimax(self, prompt: str, output_path: str):
        """Generate image by using Minimax AI"""
        minimax_config = self.config.get_minimax_config()
        if minimax_config.api_key == "":
            raise Exception("Minimax API Key not found. Please set it using /minimax-config.")
            
        api_key = minimax_config.api_key
            
        url = "https://api.minimax.io/v1/image_generation"
        headers = {"Authorization": f"Bearer {api_key}"}
        payload = {
            "model": self.minimax_image_model,
            "prompt": prompt,
            "aspect_ratio": "16:9",
            "response_format": "base64",
        }
        
        response = requests.post(url, headers=headers, json=payload, timeout=60)
        response.raise_for_status()
        
        resp_json = response.json()
        base_resp = resp_json.get("base_resp", {})
        if base_resp and base_resp.get("status_code", 0) != 0:
            raise Exception(f"Minimax API Error: {base_resp.get('status_msg', 'Unknown')} (Code: {base_resp.get('status_code')})")
            
        data = resp_json.get("data") or {}
        images = data.get("image_base64", [])
        
        if not images:
            raise Exception(f"No image returned by Minimax API. Response: {resp_json}")
            
        with open(output_path, "wb") as f:
            f.write(base64.b64decode(images[0]))
            
        return output_path
    
    def __gen_image_by_pollinations(self, prompt: str, output_path: str, sender: str = ""):
        """Generate image by using Pollinations AI"""
        encoded_prompt = requests.utils.quote(prompt)
        url = f"https://image.pollinations.ai/prompt/{encoded_prompt}"

        max_retries = 3
        for attempt in range(max_retries):
            try:
                response = requests.get(url, timeout=60)
                response.raise_for_status()
                with open(output_path, "wb") as f:
                    f.write(response.content)
                return output_path
            except requests.exceptions.RequestException as e:
                if attempt < max_retries - 1:
                    # If 429 or read timeout, wait and retry
                    if sender:
                        self.wa_tools.request_send_message(number=sender, message=f"⏳ Pollinations fallback failed: {type(e).__name__}: {e}. Retrying...", sender=sender)
                    sleep_time = (attempt + 1) * 5
                    logger.warning("[Worker:%s] Pollinations API error %s. Retrying in %ss...", sender, e, sleep_time)
                    time.sleep(sleep_time)
                else:
                    raise e
        return ""

    def request_generate_images(self, prompts: list, folder_path: str, sender: str = "") -> dict:
        """
        Generate images from a list of text prompts using Gemini (primary) and Pollinations AI (fallback).

        AI Level Instruction (Usage Guidelines):
        - Treat this as the primary method for any bulk image generation tasks.
        - Output location: Always determine a relevant folder path based on the context 
          (e.g., `playground/generated_images/topic_name`) and pass it as `folder_path`.
        - All generated images will be saved collectively inside that single specified folder.
        - The `prompts` argument must be a list of strings; each prompt will generate exactly one image.
        - Images are generated sequentially with a slight random delay (0.5s - 1.5s) to avoid rate limits.
        - Returns a formatted dict containing the status and the exact saved file paths.

        Args:
            prompts (list): A list of text descriptions/prompts for image generation.
            folder_path (str): The absolute or relative path to the directory where images should be saved.

        Returns:
            dict: A standard MCP tool response dict containing the result or error message.
        """
        try:
            resolved_folder = self.__normalize_delete_target(folder_path)
            os.makedirs(resolved_folder, exist_ok=True)
            saved_files = []

            for i, prompt in enumerate(prompts):
                # Increased base delay to avoid initial rate limiting
                time.sleep(random.uniform(2.0, 4.0))
                prompt = str(prompt).strip()
                if not prompt:
                    continue

                filename = f"image_{uuid.uuid4()}_{i+1:03d}.jpg"
                filepath = os.path.join(resolved_folder, filename)

                methods = [
                    ("Pollinations AI", lambda p, o: self.__gen_image_by_pollinations(p, o, sender)),
                ]

                if self.minimax_config.api_key != "":
                    methods.insert(0, ("Minimax", self.__gen_image_by_minimax))
                if self.google_config.api_key != "":
                    methods.insert(0, ("Gemini", self.__gen_image_by_gemini))
                
                success = False
                last_error = None
                
                for name, method in methods:
                    try:
                        res = method(prompt, filepath)
                        if res:
                            success = True
                            break
                        else:
                            logger.warning("[Worker:%s] %s generation empty.", sender, name)
                    except Exception as e:
                        last_error = e
                        logger.warning("[Worker:%s] %s generation failed: %s.", sender, name, e)
                
                if not success:
                    error_msg = f"All image generation methods failed for prompt: {prompt}. Last error: {last_error}"
                    logger.error("[Worker:%s] %s", sender, error_msg)
                    return {
                        "content": [{"type": "text", "text": error_msg}],
                        "is_error": True,
                    }

                saved_files.append(filename)
                logger.info("[Worker:%s] Saved image %s for prompt: %s", sender, filepath, prompt)

            result = (
                f"Generated {len(saved_files)} image(s) saved to folder: {resolved_folder}\n"
                f"Files: {', '.join(saved_files)}"
            )
            return {"content": [{"type": "text", "text": result, "files": [Path(resolved_folder, f) for f in saved_files]}]}

        except Exception as e:
            logger.exception("[Worker:%s] generate_images failed: %s", sender, e)
            return {
                "content": [{"type": "text", "text": f"Image generation failed: {type(e).__name__}: {e}"}],
                "is_error": True,
            }
    
    def generate_images(self, sender: str):
        def _gen(prompts: Any, folder_path: str) -> str:
            if not isinstance(prompts, list):
                raw_response = self.__llm(reasoning=True,structured_output=PromptsType).invoke(f"the prompts must return with list[str] structure: {prompts} ")
                prompts = raw_response.prompts
            self.wa_tools.request_send_message(number=sender, message="🖼️ Generating images...", sender=sender)
            res = self.request_generate_images(prompts, folder_path, sender)
            if res.get("is_error"):
                return res["content"][0]["text"]
            lists_img = res['content'][0].get('files', [])
            for img in lists_img:
                self.wa_tools.request_send_file(number=sender, file_path=str(img), caption="generated image", sender=sender)
            return res["content"][0]["text"]
        
        return StructuredTool.from_function(
            func=_gen,
            name="generate_images",
            description=    """Generate images from a list of text prompts using Pollinations AI.
AI Level Instruction (Usage Guidelines):
- Treat this as the primary method for any bulk image generation tasks.
- Pollinations AI is a free service; no API key is required.
- Output location: Always determine a relevant folder path based on the context 
    (e.g., `playground/generated_images/topic_name`) and pass it as `folder_path`.
- All generated images will be saved collectively inside that single specified folder.
- The `prompts` argument must be a list of strings; each prompt will generate exactly one image.
- IMPORTANT: If the user asks for multiple images (e.g., "generate 3 images"), you MUST provide an array of 3 distinct, separate prompt strings. DO NOT combine them into a single 'collage' or multi-part prompt. 
- Images are generated sequentially with a slight random delay to avoid rate limits.
- Returns a formatted dict containing the status and the exact saved file paths.

Args:
    prompts (list): A list of text descriptions/prompts for image generation.
    folder_path (str): The absolute or relative path to the directory where images should be saved.

Returns:
    dict: A standard MCP tool response dict containing the result or error message.""",
            handle_tool_error=True,
            args_schema=CommandInputGenerateImage,
        )
        