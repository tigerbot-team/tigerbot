# import the necessary packages

from picamera2 import Picamera2, Preview
import sys
import time


class CommandServer(object):

    def __init__(self):
        self.cam = Picamera2()
        self.last_picture_index = 0
        self.file_name_format = 'pic%03d.jpg'

        camera_config = self.cam.create_still_configuration()
        self.cam.configure(camera_config)
        
    def start(self):
        print("Call start")
        self.cam.start()

    def serve_commands(self):
        for command in sys.stdin:
            command = command.strip()
            if command == "take-picture":
                result = self.take_picture()
            else:
                result = "Unknown command: " + command

            print("RESULT:", result, flush=True)
        
    def take_picture(self):
        picture_index = self.last_picture_index + 1
        file_name = self.file_name_format % picture_index
        print("Call capture_file", file_name)
        self.cam.capture_file(file_name)
        self.last_picture_index = picture_index
        return file_name


cs = CommandServer()
cs.start()
cs.serve_commands()
