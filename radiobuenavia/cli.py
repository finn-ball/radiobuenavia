import logging
import os
import tempfile

import dropbox
import toml

from . import tools
from .audacity import PipeClient
from .dropbox import DropBoxClient

print("""This software is distributed under the GNU GENERAL PUBLIC LICENSE agreement.
This software comes with absolutely no warranty or liability.
More information can be found in the LICENSE file.""")

def cli():
    logging.basicConfig(level=logging.INFO)
    print(__art)
    logging.info("Starting up...")

    try:
        data = toml.load("./config.toml")
        app_key = data["auth"]["app_key"]
        app_secret = data["auth"]["app_secret"]
        refresh_token = data["auth"]["refresh_token"]
        preprocess_live = data["paths"]["preprocess_live"]
        preprocess_prerecord = data["paths"]["preprocess_prerecord"]
        postprocess_soundcloud = data["paths"]["postprocess_soundcloud"]
        postprocess_archive = data["paths"]["postprocess_archive"]
        jingles = data["paths"]["jingles"]
    except Exception as e:
        logging.error(e)
        logging.error("Hit enter to leave.")
        input()
        return

    if jingles != []:
        logging.info("Found jingles: {}".format(jingles))

    for j in jingles:
        if not os.path.exists(j):
            logging.error("Could not find jingle: {}".format(j))
            logging.error("Hit enter to leave.")
            input()
            return

    err = True
    audacity = None
    try:
        audacity = PipeClient()
        run(app_key, app_secret, refresh_token,
            preprocess_live, postprocess_soundcloud, postprocess_archive, audacity, jingles, True)
        run(app_key, app_secret, refresh_token,
            preprocess_prerecord, postprocess_soundcloud, postprocess_archive, audacity, jingles, False)
        err = False
    except dropbox.exceptions.AuthError as e:
        logging.error("Are the credentials correct?")
        logging.error(e)
    except FileNotFoundError as e:
        logging.error("Potentially cannot find file: ")
        logging.error(e)
    except Exception as e:
        logging.error(e.__class__)
        logging.error(e)
    finally:
        if isinstance(audacity, PipeClient):
            try:
                audacity.close()
            except Exception as e:
                logging.error(e)
                logging.error("Couldn't close pipe.")
        if err:
            print("")
            logging.error("Try restarting audacity and rerunning the script.")
        print("\nRemember to close audacity to free up memory.")
        print("\nDone (hit enter)")
        input()


def run(app_key, app_secret, refresh_token, preprocess, archive, soundcloud, audacity, jingles, live):
    tmp_dir = os.path.join(tempfile.gettempdir(), "rbv")
    if not os.path.isdir(tmp_dir):
        os.mkdir(tmp_dir)
    dbx = DropBoxClient(
        app_key,
        app_secret,
        refresh_token,
        preprocess,
        archive,
        soundcloud,
    )
    preproc = dbx.list_files_to_process()

    if preproc == []:
        logging.info("\n\n\tNo new files to process!")
        return

    print("\nFiles to process ({}):\n".format(len(preproc)))
    for f in preproc:
        print("{} -> {}".format(f.name, dbx.rename_file(f)))

    choice = input("\nProceed? (Y/n)")
    if not (choice == "Y" or choice == "y"):
        logging.info("Goodbye!")
        return

    for f in preproc:
        name = dbx.rename_file(f)
        print("\n\t Processing:\n\t\t{}\n".format(name))
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
            audacity.process(audacity_import, audacity_export, live)
        except RuntimeError as e:
            logging.error("Failed to execute command.")
            raise e
        logging.info("Done!")

        artist = tools.get_artist(name)
        logging.info("Setting artist name and potentially changing bitrate.")
        tools.process_metadata_and_bitrate(audacity_export, artist, jingles)

        try:
            logging.info("Uploading... {}".format(name))
            dbx.upload_file_soundcloud(audacity_export, name)
            logging.info("Copying to archive... {}".format(name))
            dbx.copy_to_archive(name)
        except dropbox.exceptions.ApiError as e:
            if isinstance(e.error, dropbox.files.UploadError):
                logging.error(
                    "Couldn't upload {}, does the file already exist?".format(name))
                raise e
        except ValueError as e:
            logging.error("File size likely larger than 150Mb.")
            raise e
        logging.info("Uploaded \"%s\"", name)


if __name__ == "__main__":
    cli()

__art = r'''
You are watching... Radio Buena Via...
           _ . - = - . _
       . "  \  \   /  /  " .
     ,  \                 /  .
   . \   _,.--~=~"~=~--.._   / .
  ;  _.-"  / \ !   ! / \  "-._  .
 / ,"     / ,` .---. `, \     ". \
/.'   `~  |   /:::::\   |  ~`   '.\
\`.  `~   |   \:::::/   | ~`  ~ .'/
 \ `.  `~ \ `, `~~~' ,` /   ~`.' /
  .  "-._  \ / !   ! \ /  _.-"  .
   ./    "=~~.._  _..~~=`"    \.
     ,/         ""          \,
       . _/             \_ .
          " - ./. .\. - "
'''
