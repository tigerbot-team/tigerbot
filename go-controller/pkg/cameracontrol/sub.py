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

        # No contours => tiny contour area, which will in turn mean
        # negligible confidence.  First return value needs to be
        # non-zero because the receiving code takes its logarithm.
        if len(contours) == 0:
            return "1 0 0"

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

    def do_white_line(self):
        file_name = self._take_picture()
        return self._white_line(file_name, 70)

    def do_test_white_line(self):
        result0 = self._white_line("test-white-line.jpg", 0)
        result70 = self._white_line("test-white-line.jpg", 70)
        print("Result with 0% blinkers", result0)
        print("Result with 70% blinkers", result70)
        return result70

    # `blinkers`: How much, out of the 200 total width, to ignore on
    # the left and hand edges of the picture.
    def _white_line(self, filename, blinkers):
        img = cv2.imread(filename)
        rows, columns, _ = img.shape

        # Pi camera takes photos at 4608 x 2592, which is stupidly
        # high res, but too dangerous to change at this stage.  We
        # will sample 20 rows from the lower half of the photo, and
        # for each of those rows work out where the white line is.
        centres = []
        jvalues = []
        ymins = []
        ymaxs = []
        for j in range(20):
            row = rows - 1 - j * (rows // 40)
            # Horizontally, use 200 samples across the row.  In
            # calibration photos the white line occupies approx 1/25
            # of the photo width, so this should give us 8 samples in
            # the white area.
            dx = (columns - 1) // 200
            yrow = []
            pending_first_value = True
            for i in range(blinkers, 200 - blinkers):
                col = i * dx
                b = img.item(row, col, 0)
                g = img.item(row, col, 1)
                r = img.item(row, col, 2)
                # Grayscale conversion per
                # https://docs.opencv.org/4.x/de/d25/imgproc_color_conversions.html#color_convert_rgb_gray
                y = 0.299 * r + 0.587 * g + 0.114 * b
                yrow.append(y)
                if pending_first_value:
                    ymin = y
                    ymax = y
                    pending_first_value = False
                else:
                    if y < ymin:
                        ymin = y
                    if y > ymax:
                        ymax = y
            yavg = (ymin + ymax) / 2
            count = 0
            moment = 0
            for i in range(blinkers, 200 - blinkers):
                y = yrow[i-blinkers]
                if y > yavg:
                    ynorm = (y - ymin) / (ymax - ymin)
                    count += ynorm
                    moment += ynorm * i
            if count > 0:
                centres.append(moment / count)
                jvalues.append(j)
                ymins.append(ymin)
                ymaxs.append(ymax)
        print(centres)
        print(jvalues)
        print(ymins)
        print(ymaxs)

        # Fit a straight line to those centres.
        fit = np.polyfit(np.array(jvalues),
                         np.array(centres),
                         1)
        print(fit)

        return "%f %f" % (fit[0], fit[1])


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
