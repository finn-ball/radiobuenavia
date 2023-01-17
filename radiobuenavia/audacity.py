import os
import tempfile

class PipeClient:
    def __init__(self):
        # if sys.platform == 'win32':
        #     self.__to_name = '\\\\.\\pipe\\ToSrvPipe'
        #     self.__from_name = '\\\\.\\pipe\\FromSrvPipe'
        # else:
        self.__to_name = os.path.join(
            tempfile.gettempdir(),
            "audacity_script_pipe.to.{}".format(str(os.getuid()))
        )
        if not os.path.exists(self.__to_name):
            raise FileNotFoundError(self.__to_name)
        self.__from_name = os.path.join(
            tempfile.gettempdir(),
            "audacity_script_pipe.from.{}".format(str(os.getuid()))
        )
        if not os.path.exists(self.__from_name):
            raise FileNotFoundError(self.__from_name)

    def send_command(self, command):
        """Send a single command."""
        with open(self.__to_name, 'w') as f:
            f.write(command + os.linesep)
            f.flush()

    def get_response(self) -> str:
        """Return the command response."""
        result = ''
        line = ''
        with open(self.__from_name, 'rt') as f:
            line = f.readline()
            result += line
            while line != os.linesep and len(result) > 0:
                line = f.readline()
                result += line
        return result

    def do_command(self, command) -> str:
        """Send one command, and return the response."""
        self.send_command(command)
        response = self.get_response()
        if "BatchCommand finished: Failed!" in response:
            raise RuntimeError(response)
        return response


    def process(self, import_path: str, export_path: str):
        """Process the file by importing, running filters and exporting."""
        self.do_command("Import2: Filename={}".format(import_path))
        self.do_command('SelectAll: ')
        self.do_command('ChangePitch: Percentage=190')
        self.do_command("Export2: Filename={}".format(export_path))
        self.do_command('TrackClose:')
