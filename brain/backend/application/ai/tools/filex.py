

import shutil
import os
from pathlib import Path

class Filex: 
    def __init__(self):
        self._playground_root = self.__resolve_playground_root()

    def __resolve_playground_root(self) -> Path:
        """Resolve repository playground root for file guard checks."""
        current_file = Path(__file__).resolve()

        # Prefer existing playground discovered from cwd first for runtime flexibility.
        cwd_candidate = (Path.cwd().resolve() / "playground").resolve()
        if cwd_candidate.exists():
            return cwd_candidate

        for parent in current_file.parents:
            candidate = (parent / "playground").resolve()
            if candidate.exists():
                return candidate

        # Fallback path when playground is not created yet.
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
            raise FileNotFoundError(f"File or directory not found: {file_path}")
            
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