import logging
import os

import dropbox
import toml

from .audacity import PipeClient
from .dropbox import DropBoxClient


def cli():
    logging.basicConfig(level=logging.INFO)
    logging.info("Starting up")

    data = toml.load("./config.toml")
    app_key = data["auth"]["app_key"]
    app_secret = data["auth"]["app_secret"]
    refresh_token = data["auth"]["refresh_token"]
    preprocess = data["paths"]["preprocess"]
    postprocess = data["paths"]["postprocess"]
    try:
        run(app_key, app_secret, refresh_token, preprocess, postprocess)
    except dropbox.exceptions.AuthError as e:
        logging.error(e)
        return -1

def run(app_key, app_secret, refresh_token, preprocess, postprocess):
    dbx = DropBoxClient(
        app_key,
        app_secret,
        refresh_token,
        preprocess,
        postprocess,
    )
    preproc = dbx.list_files_to_process()

    if preproc == []:
        logging.info("No new files to process!")
        return

    print("\nFiles to process ({}):\n".format(len(preproc)))
    for f in preproc:
        print("{} -> {}".format(f.name, dbx.rename_file(f)))

    choice = input("\nProceed? (Y/n)")
    if not(choice == "Y" or choice == "y"):
        logging.info("Goodbye!")
        return

    audacity = PipeClient()

    for f in preproc:
        name = dbx.rename_file(f)
        local_path = "/tmp/{}".format(name)
        logging.info("Downloading %s to %s", f.name, local_path)
        dbx.download_file(local_path, f.path_lower)

        # Audacity struggles to import files with spaces so we rename it
        rename_import = "/tmp/im-{}".format(name.replace(" ", "-"))
        rename_export = "/tmp/ex-{}".format(name.replace(" ", "-"))
        os.rename(local_path, rename_import)

        logging.info("Processing...")
        try:
            audacity.process(rename_import, rename_export)
        except RuntimeError as e:
            logging.error("Failed to execute command.")
            logging.error(e)
            return -1
        logging.info("Done!")

        os.rename(rename_export, local_path)

        try:
            logging.info("Attempting to upload \"%s\"", name)
            dbx.upload_file(local_path, name)
        except dropbox.exceptions.ApiError as e:
            if isinstance(e.error, dropbox.files.UploadError):
                logging.error("Couldn't upload \"%s\", does the file already exist?", name)
                logging.error(e)
                return -1
        except ValueError as e:
            logging.error("File size likely larger than 150Mb.")
            logging.error(e)
        logging.info("Uploaded \"%s\"", name)

if __name__ == "__main__":
    cli()
