import os
import sys
import tempfile
if sys.platform == 'win32':
    import win32pipe, win32file

class PipeClient:
    def __init__(self):
        if sys.platform == 'win32':
            self.__write_pipe_path = r'\\.\pipe\ToSrvPipe'
            self.__read_pipe_path =  r'\\.\pipe\FromSrvPipe'
            self.__write_pipe_id = win32pipe.CreateNamedPipe(
                self.__write_pipe_path,
                win32pipe.PIPE_ACCESS_DUPLEX,
                win32pipe.PIPE_TYPE_MESSAGE | win32pipe.PIPE_READMODE_MESSAGE | win32pipe.PIPE_WAIT,
                1, 65536, 65536,
                0,
                None)
            self.__read_pipe_id = win32file.CreateFile(
                r'\\.\pipe\FromSrvPipe',
                win32file.GENERIC_READ | win32file.GENERIC_WRITE,
                0,
                None,
                win32file.OPEN_EXISTING,
                0,
                None
            )
            self.__write_pipe=open(self.__write_pipe_path, 'w')
            win32pipe.SetNamedPipeHandleState(
                self.__read_pipe_id, win32pipe.PIPE_READMODE_MESSAGE, None, None)
        else:
            self.__write_pipe_path = os.path.join(
                tempfile.gettempdir(),
                "audacity_script_pipe.to.{}".format(str(os.getuid()))
            )
            self.__read_pipe_path = os.path.join(
                tempfile.gettempdir(),
                "audacity_script_pipe.from.{}".format(str(os.getuid()))
            )
            self.__write_pipe=open(self.__write_pipe_path, 'w')
        if not os.path.exists(self.__write_pipe_path):
            raise FileNotFoundError(self.__write_pipe_path)
        self.__write_pipe.flush()

    def close(self):
        if sys.platform == 'win32':
            win32file.CloseHandle(self.__read_pipe_id)
            win32file.CloseHandle(self.__write_pipe_id)
        self.__write_pipe.close()

    def send_command(self, command):
        """Send a single command."""
        self.__write_pipe.write(command + os.linesep)
        self.__write_pipe.flush()

    def get_response(self) -> str:
        if sys.platform == 'win32':
            handle = self.__read_pipe_id
            result = ""
            line = str(win32file.ReadFile(handle, 64*1024)[1])
            while "BatchCommand finished" not in line:
                line = str(win32file.ReadFile(handle, 64*1024)[1])
                result += line
            return result
        else:
            result = ""
            with open(self.__read_pipe_path, 'rt') as f:
                line = f.readline()
                result = line
                while line != "\n" and len(result) > 0:
                    line = f.readline()
                    result += line
            return result

    def do_command(self, command) -> str:
        """Send one command, and return the response."""
        print(command)
        self.send_command(command)
        response = self.get_response()
        if "BatchCommand finished: Failed!" in response:
            msg = "{}\n{}".format(command, response)
            raise RuntimeError(msg)
        return response

    def process(self, import_path: str, export_path: str, live: bool):
        """Process the file by importing, running filters and exporting."""
        self._import(import_path)
        self._select_all()
        self._process_live() if live else self._process_prerecord()
        self._export(export_path)
        self._close_track()

    def _import(self, path):
        self.do_command("Import2: Filename={}".format(path))

    def _select_all(self):
        self.do_command("SelectAll: ")

    def _process_prerecord(self):
        self._compressor(
            threshold=-12,
            noise_floor=-40,
            ratio=2,
            attack_time=3,
            release_time=10,
            normalize=True,
            use_peak=True
        )
        self._limiter(
            type="SoftLimit",
            gain_l=0,
            gain_r=0,
            thresh=-4,
            hold=6.2,
            makeup="No",
        )

    def _process_live(self):
        self._normalizer(
            peak_level=-0.3,
            apply_gain=True,
            remove_dc_offset=True,
            stereo_independent=False,
        )

    def _compressor(self, threshold: int, noise_floor: int, ratio: int, attack_time: int, release_time: int, normalize: bool, use_peak: bool):
        self.do_command("Compressor: Threshold={} NoiseFloor={} Ratio={} AttackTime={} ReleaseTime={} Normalize={} UsePeak={}".format(
            threshold,
            noise_floor,
            ratio,
            attack_time,
            release_time,
            normalize,
            use_peak
        ))

    def _limiter(self, type: str, gain_l: int, gain_r: int, thresh: int, hold: int, makeup: str):
        self.do_command("Limiter: type={} gain-L={} gain-R={} thresh={} hold={} makeup={}".format(
            type,
            gain_l,
            gain_r,
            thresh,
            hold,
            makeup
        ))

    def _normalizer(self, peak_level: int, apply_gain: bool, remove_dc_offset: bool, stereo_independent: bool):
        self.do_command("Normalize: PeakLevel={} ApplyGain={} RemoveDcOffset={} StereoIndepend={}".format(
            peak_level,
            apply_gain,
            remove_dc_offset,
            stereo_independent
        ))

    def _export(self, path):
        self.do_command("Export2: Filename={} NumChannels=2".format(path))

    def _close_track(self):
        self.do_command("TrackClose:")
