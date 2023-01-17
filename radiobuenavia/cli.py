import logging
import os
import tempfile

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
        logging.error("Are the credentials correct?")
        logging.error(e)
        return -1

def run(app_key, app_secret, refresh_token, preprocess, postprocess):
    tmp_dir = os.path.join(tempfile.gettempdir(), "rbv")
    if not os.path.isdir(tmp_dir):
        os.mkdir(tmp_dir)
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
        # Audacity struggles to import files with spaces so we rename it
        audacity_import = os.path.join(
            tmp_dir,
            "im-{}".format(name.replace(" ", "-"))
        )
        audacity_export = os.path.join(
            tmp_dir,
            "ex-{}".format(name.replace(" ", "-"))
        )
        logging.info("Downloading %s to %s", f.name, audacity_import)
        dbx.download_file(audacity_import, f.path_lower)

        logging.info("Processing...")
        try:
            audacity.process(audacity_import, audacity_export)
        except RuntimeError as e:
            logging.error("Failed to execute command.")
            logging.error(e)
            return -1
        logging.info("Done!")

        try:
            logging.info("Attempting to upload \"%s\"", name)
            dbx.upload_file(audacity_export, name)
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
