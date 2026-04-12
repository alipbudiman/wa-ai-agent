

import shutil
import os

class Filex: 
    def __init__(self):
        pass

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
        res = shutil.make_archive(
            base_name=base_name,
            format=format, 
            root_dir=file_path
        )
        return res
    
    def delete_file_dir(self, file_path: str) -> str:
        """
        Delete file and directory
        
        Args:
            file_path (str): The path to the file or directory to delete.
        
        Returns:
            str: A message indicating the file or directory has been deleted.
        """

        # gurad
        if file_path.startswith("/playground"):
            return "You can't delete files from /playground, ask the user to delete the file from the playground"

        os.remove(file_path)
        return f"Compressed file deleted: {file_path}"