import os

import pydub
from pydub.utils import mediainfo

def process_metadata_and_bitrate(path: str, artist: str):
    """Sets the track name as the artist and lowers the bitrate for shows over 90mins"""
    sound = pydub.AudioSegment.from_file(path)
    bitrate=_get_bitrate(path, sound)
    sound.export(path, format="mp3", bitrate=bitrate,  tags={"artist": artist})


def get_artist(path: str) -> str:
    """Returns the title of the track to be the artist name"""
    ext = path.split(".")[-1]
    return os.path.basename(path).rstrip(".{}".format(ext))

def _get_bitrate(path: str, sound: pydub.AudioSegment) -> str:
    """If the track is longer than 90 minutes, lower the bitrate."""
    original_bitrate = mediainfo(path)['bit_rate']
    if sound.duration_seconds > 90 * 60:
        return "210k"
    else:
        return original_bitrate
