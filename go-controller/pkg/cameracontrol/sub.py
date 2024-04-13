# import the necessary packages

import cv2
import numpy as np
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
            attr = "do_" + command.replace("-", "_")
            method = getattr(self, attr, None)
            if method is not None:
                result = method()
            else:
                result = "Unknown command: " + command
            print("RESULT:", result, flush=True)

    def _take_picture(self):
        picture_index = self.last_picture_index + 1
        file_name = self.file_name_format % picture_index
        print("Call capture_file", file_name)
        self.cam.capture_file(file_name)
        self.last_picture_index = picture_index
        return file_name

    def do_take_picture(self):
        return self._take_picture()

    def do_id_block_colour(self):
        file_name = self._take_picture()
        return self._id_block_colour(file_name)

    def do_test_id(self):
        return self._id_block_colour("test-id.jpg")

    def _id_block_colour(self, filename):
        img = cv2.imread(filename)
        print("Shape =", img.shape)
        hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)

        bestColour = ""
        bestArea = 0
        for colour, range in hue_ranges.items():
            if range.max > range.min:
                mask = self._hsv_mask(hsv, range.min, range.max)
            else:
                mask = (self._hsv_mask(hsv, range.min, 180) +
                        self._hsv_mask(hsv, 0, range.max))

            # Apply two iterations each of erosion and dilation, to
            # remove noise.
            mask = cv2.erode(mask, None, iterations=2)
            mask = cv2.dilate(mask, None, iterations=2)

            # Find contours of the mask.
            contours, _ = cv2.findContours(mask.copy(),
                                           cv2.RETR_EXTERNAL,
                                           cv2.CHAIN_APPROX_SIMPLE)
            largestContour = max(contours, key=cv2.contourArea)
            largestContourArea = cv2.contourArea(largestContour)

            print(colour, ": largestContourArea", largestContourArea)

            if bestColour == "" or largestContourArea > bestArea:
                bestColour = colour
                bestArea = largestContourArea

        return bestColour

    def _hsv_mask(self, hsv, min, max):
        lower = np.array([min, 50, 50])
        upper = np.array([max, 255, 255])
        return cv2.inRange(hsv, lower, upper)


class HueRange(object):
    def __init__(self, min, max):
        self.min = min
        self.max = max


hue_ranges = {
    "red": HueRange(165, 10),
    "green": HueRange(45, 100),
    "blue": HueRange(100, 120),
}


cs = CommandServer()
cs.start()
cs.serve_commands()
