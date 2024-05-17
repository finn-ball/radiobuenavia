import logging
import os
from typing import List

import dropbox
from dropbox.files import CommitInfo, FileMetadata, UploadSessionCursor

CHUNK_SIZE = 4 * 1024 * 1024


class DropBoxClient():
    def __init__(self, app_key: str, app_secret: str, refresh_token: str, preprocess_path: str, postprocess_archive_path: str, postprocess_soundcloud_path: str):
        self.__dbx = dropbox.Dropbox(
            app_key=app_key,
            app_secret=app_secret,
            oauth2_refresh_token=refresh_token
        )
        self.__preprocess_path = preprocess_path
        self.__postprocess_archive_path = postprocess_archive_path
        self.__postprocess_soundcloud_path = postprocess_soundcloud_path

    def list_files(self, path: str) -> List[FileMetadata]:
        """List all files in the directory."""
        result = self.__dbx.files_list_folder(path)
        entries = [p for p in result.entries if isinstance(p, FileMetadata)]
        # TODO: check this mechanism
        while result.has_more:
            result = self.__dbx.files_list_folder_continue(result.cursor)
            entries.extend(
                [p for p in result.entries if isinstance(p, FileMetadata)])
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

    def upload_file_soundcloud(self, local_path: str, name: str) -> None:
        remote_path = self._get_remote_path_soundcloud(name)
        self.upload_file(local_path, remote_path)

    def _get_remote_path_soundcloud(self, name: str):
        return self._get_remote_path(self.__postprocess_soundcloud_path, name)

    def _get_remote_path_archive(self, name: str):
        return self._get_remote_path(self.__postprocess_archive_path, name)

    def _get_remote_path(self, path:str, name: str):
        return "{}/{}".format(path, name)

    def copy_to_archive(self, name: str):
        remote_path_from = self._get_remote_path_soundcloud(name)
        remote_path_to = self._get_remote_path_archive(name)
        self.copy(remote_path_from, remote_path_to)

    def copy(self, from_path: str, to_path: str):
        self.__dbx.files_copy_v2(from_path, to_path)

    def upload_file(self, local_path: str, remote_path: str) -> None:
        """Uploads files using the session method."""
        file_size = os.path.getsize(local_path)
        n_total = file_size / CHUNK_SIZE
        n = 0
        with open(local_path, 'rb') as f:
            if file_size <= CHUNK_SIZE:
                self.__dbx.files_upload(f.read(), remote_path)
            else:
                upload_session_start_result = self.__dbx.files_upload_session_start(
                    f.read(CHUNK_SIZE)
                )
                cursor = UploadSessionCursor(
                    session_id=upload_session_start_result.session_id,
                    offset=f.tell()
                )
                commit = CommitInfo(path=remote_path)
                while f.tell() < file_size:
                    if ((file_size - f.tell()) <= CHUNK_SIZE):
                        self.__dbx.files_upload_session_finish(
                            f.read(CHUNK_SIZE),
                            cursor,
                            commit
                        )
                    else:
                        self.__dbx.files_upload_session_append_v2(
                            f.read(CHUNK_SIZE),
                            cursor,
                        )
                        cursor.offset = f.tell()
                    n+=1
                    logging.info("{} - {}%".format(
                        remote_path,
                        round((n/n_total)*100)
                    ))

    def list_files_to_process(self) -> List[FileMetadata]:
        """List all files that can be processed."""
        preproc = []
        postproc = [f.name for f in self.list_files(self.__postprocess_archive_path)]
        for f in self.list_files(self.__preprocess_path):
            if not self.rename_file(f) in postproc:
                preproc.append(f)
        return preproc
