import logging
import os
import random
from typing import List

import pydub
from pydub.utils import mediainfo

def process_metadata_and_bitrate(path: str, artist: str, jingles: List[str]):
    """Sets the track name as the artist and lowers the bitrate for shows over 90mins"""
    sound = pydub.AudioSegment.from_file(path)
    # Get bitrate now before potentially adding jingle.
    bitrate=_get_bitrate(path, sound)
    # If there is a jingle, append one of them randomly.
    if jingles != []:
        jingle = jingles[random.randrange(0, len(jingles))]
        logging.info("Adding jingle: {}".format(jingle))
        j = pydub.AudioSegment.from_file(jingle)
        sound = j + sound
    logging.info("Re-exporting...")
    sound.export(path, format="mp3", bitrate=bitrate,  tags={"artist": artist})


def get_artist(path: str) -> str:
    """Returns the title of the track to be the artist name"""
    ext = path.split(".")[-1]
    return os.path.basename(path).rstrip(".{}".format(ext))

def _get_bitrate(path: str, sound: pydub.AudioSegment) -> str:
    """If the track is longer than 90 minutes, lower the bitrate."""
    original_bitrate = mediainfo(path)['bit_rate']
    logging.info("Current bitrate: {}".format(original_bitrate))
    if sound.duration_seconds > 90 * 60:
        logging.info("Bitrate changed to 210k.")
        return "210k"
    else:
        logging.info("Bitrate unchanged.")
        return original_bitrate
