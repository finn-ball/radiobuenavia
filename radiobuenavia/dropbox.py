import os
from typing import List

import dropbox
from dropbox.files import FileMetadata


class DropBoxClient():
    def __init__(self, app_key:str, app_secret: str, refresh_token: str, preprocess_path: str, postprocess_path: str):
        self.__dbx = dropbox.Dropbox(
            app_key = app_key,
            app_secret=app_secret,
            oauth2_refresh_token=refresh_token
        )
        self.__preprocess_path = preprocess_path
        self.__postprocess_path = postprocess_path

    def list_files(self, path: str) -> List[FileMetadata]:
        """List all files in the directory."""
        result = self.__dbx.files_list_folder(path)
        entries = [p for p in result.entries if isinstance(p, FileMetadata)]
        # TODO: check this mechanism
        while result.has_more:
            result = self.__dbx.files_list_folder_continue(result.cursor)
            entries.extend([p for p in result.entries if isinstance(p, FileMetadata)])
        return entries

    def rename_file(self, file: FileMetadata) -> str:
        """Rename the file to be compliant with Buena Vida."""
        # Get the file extension, e.g "mp3"
        ext = file.name.split(".")[-1]
        # Strip the extension for now
        name = file.name.rstrip(".{}".format(ext))
        # Format needs to be "Show Name - Radio Buena Vida DD.MM.YY"
        name = "{} - Radio Buena Vida {}.{}".format(
            name,
            file.client_modified.strftime("%d.%m.%y"),
            ext
        )
        return name

    def download_file(self, local_path: str, download_path: str) -> None:
        """Downloads the file."""
        self.__dbx.files_download_to_file(local_path, download_path)

    # TODO: Upload limit is 150Mb...
    def upload_file(self, local_path: str, name: str) -> None:
        """Uploads files smaller than 150Mb."""
        remote_path = "{}/{}".format(self.__postprocess_path, name)
        # We may need a way to upload larger files
        if self.__file_size_check(local_path):
            raise ValueError
        with open(local_path, 'rb') as f:
            self.__dbx.files_upload(f.read(), remote_path)

    def list_files_to_process(self) -> List[FileMetadata]:
        """List all files that can be processed."""
        preproc = []
        postproc = [f.name for f in self.list_files(self.__postprocess_path)]
        for f in self.list_files(self.__preprocess_path):
            if not self.rename_file(f) in postproc:
                preproc.append(f)
        return preproc

    def __file_size_check(self, path) -> bool:
        file_size = os.path.getsize(path)
        return file_size >= 157286400
