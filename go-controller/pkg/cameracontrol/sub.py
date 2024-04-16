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

    def do_find_zombies(self):
        im = self.cam.capture_array()
        crop = im[1000:, 1000:3608]

        frame_128 = cv2.resize(crop, (128,128))
        frame_128 = cv2.rotate(frame_128, cv2.ROTATE_90_CLOCKWISE)
        frame_red_blue = cv2.cvtColor(frame_128, cv2.COLOR_RGB2BGR)
        frame_rgb565 = cv2.cvtColor(frame_red_blue, cv2.COLOR_RGB2BGR565)
        with open('/dev/fb0', 'rb+') as buf:
            buf.write(frame_rgb565)

    def do_take_picture(self):
        return self._take_picture()

    def do_id_block_colour(self):
        file_name = self._take_picture()
        return self._id_block_colour(file_name)

    def do_test_id(self):
        return self._id_block_colour("test-id.jpg")

    def do_id_mine(self):
        file_name = self._take_picture()
        return self._id_mine(file_name)

    def do_test_mine(self):
        return self._id_mine("test-mine.jpg")

    def _id_block_colour(self, filename):
        img = cv2.imread(filename)
        print("Shape =", img.shape)
        hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)

        bestColour = ""
        bestArea = 0
        for colour, range in hue_ranges.items():
            contours = self._contours_in_hue_range(hsv, range)
            largestContour = max(contours, key=cv2.contourArea)
            largestContourArea = cv2.contourArea(largestContour)

            print(colour, ": largestContourArea", largestContourArea)

            if bestColour == "" or largestContourArea > bestArea:
                bestColour = colour
                bestArea = largestContourArea

        return bestColour

    def _id_mine(self, filename):
        img = cv2.imread(filename)
        print("Shape =", img.shape)
        hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)

        contours = self._contours_in_hue_range(hsv, HueRange(165, 10))
        largestContour = max(contours, key=cv2.contourArea)
        largestContourArea = cv2.contourArea(largestContour)
        M = cv2.moments(largestContour)
        rows, columns, _ = img.shape
        x = (M["m10"] / M["m00"]) / columns
        y = (M["m01"] / M["m00"]) / rows


        print("largestContourArea", largestContourArea)
        print("X", x)
        print("Y", y)

        cv2.drawContours(img, contours, -1, (255, 0, 0), 3)
        cv2.drawContours(img, [largestContour], 0, (0, 255, 0), 3)
        contourFileName = filename.replace('.jpg', '-contour.jpg')
        cv2.imwrite(contourFileName, img)

        return "%f %f %f" % (largestContourArea, x, y)

    def _hsv_mask(self, hsv, min, max):
        lower = np.array([min, 100, 100])
        upper = np.array([max, 255, 255])
        return cv2.inRange(hsv, lower, upper)

    def _contours_in_hue_range(self, hsv, hue_range):
        if hue_range.max > hue_range.min:
            mask = self._hsv_mask(hsv, hue_range.min, hue_range.max)
        else:
            mask = (self._hsv_mask(hsv, hue_range.min, 180) +
                    self._hsv_mask(hsv, 0, hue_range.max))

        # Apply two iterations each of erosion and dilation, to
        # remove noise.
        mask = cv2.erode(mask, None, iterations=2)
        mask = cv2.dilate(mask, None, iterations=2)

        # Find contours of the mask.
        contours, _ = cv2.findContours(mask.copy(),
                                       cv2.RETR_EXTERNAL,
                                       cv2.CHAIN_APPROX_SIMPLE)
        return contours


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
