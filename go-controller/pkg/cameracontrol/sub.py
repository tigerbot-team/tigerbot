# import the necessary packages

import sys

import cv2
import numpy as np
from picamera2 import Picamera2, Preview
import sys
import time
import json
import os


C = {
    "escape": {
        "red": [165, 10, 100, 255, 60, 255],
        "green": [45, 100, 10, 255, 0, 255],
        "blue": [100, 120, 90, 255, 80, 255],
    },
    "mine": [165, 10, 100, 255, 60, 255],
    "eco": {
        "red": [165, 10, 100, 255, 60, 255],
        "green": [45, 100, 10, 255, 0, 255],
        "blue": [100, 120, 90, 255, 80, 255],
        "yellow": [22, 42, 100, 255, 130, 255],
    },
}


class CommandServer(object):

    def __init__(self):
        self.cam = Picamera2()
        self.last_picture_index = 0
        self.file_name_format = 'pic%03d.jpg'

        camera_config = self.cam.create_still_configuration()
        self.cam.configure(camera_config)
        global C

        # Write out default config.
        with open('config-default.json', 'w') as f:
            f.write(json.dumps(C))

        if os.path.isfile('config.json'):
            with open('config.json', 'r') as f:
                C = json.load(f)

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

        self.show_image_on_screen(crop)

    def show_image_on_screen(self, im):
        frame_128 = cv2.resize(im, (128,128))
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

    def do_show_aim_point(self):
        im = self.cam.capture_array()
        rows, cols = im.shape[:2]
        aim_x_offset = 40
        aim_y_offset = 160
        crop = im[rows//2-200+aim_y_offset:rows//2+200+aim_y_offset,
        cols//2-200+aim_x_offset:cols//2+200+aim_x_offset]
        cv2.rectangle(crop, (180, 180), (220, 220), (255, 0, 0), 5)
        self.show_image_on_screen(cv2.cvtColor(crop, cv2.COLOR_RGB2BGR))

    def _id_block_colour(self, filename):
        img = cv2.imread(filename)
        print("Shape =", img.shape)
        hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)

        result = ""
        for colour, minmax in C["escape"].items():
            contours = self._contours_in_hue_range(hsv, minmax)
            largestContour = max(contours, key=cv2.contourArea)
            largestContourArea = cv2.contourArea(largestContour)

            print(colour, ": largestContourArea", largestContourArea)

            if result != "":
                result += " "
            result += f"{colour} {largestContourArea}"

        return result

    def _id_mine(self, filename):
        img = cv2.imread(filename)
        print("Shape =", img.shape)
        hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)

        contours = self._contours_in_hue_range(hsv, C["mine"])

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

    def do_test_barrels(self):
        mask = cv2.imread("/home/nell/piwars/barrel-calibration/mask.jpg",
                          cv2.IMREAD_GRAYSCALE)
        cv2.imwrite("/home/nell/piwars/barrel-calibration/mask-grey.jpg", mask)
        rows, cols = mask.shape
        print("mask: rows", rows, "cols", cols)
        for pic in range(1, 19):
            filename = "/home/nell/piwars/barrel-calibration/pic%03d.jpg" % pic
            print("Processing", filename)
            img = cv2.imread(filename)
            img = cv2.bitwise_and(img, img, mask=mask)
            cv2.imwrite(filename.replace('.jpg', '-masked.jpg'), img)
            hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
            for colour in ["red", "green"]:
                print("Looking for", colour)
                r = C["eco"][colour]
                contours = self._contours_in_hue_range(hsv, r)

                # No contours => tiny contour area, which will in turn mean
                # negligible confidence.  First return value needs to be
                # non-zero because the receiving code takes its logarithm.
                if len(contours) == 0:
                    print("No", colour, "contours found")
                    continue

                largestContour = max(contours, key=cv2.contourArea)
                largestContourArea = cv2.contourArea(largestContour)
                M = cv2.moments(largestContour)
                rows, columns, _ = img.shape
                x = (M["m10"] / M["m00"]) / columns
                y = (M["m01"] / M["m00"]) / rows

                print(colour, "largestContourArea", largestContourArea)
                print(colour, "X", x)
                print(colour, "Y", y)

                c = img.copy()
                cv2.drawContours(c, contours, -1, (255, 0, 0), 6)
                cv2.drawContours(c, [largestContour], 0, (0, 255, 0), 6)
                contourFileName = filename.replace('.jpg', '-'+colour+'.jpg')
                cv2.imwrite(contourFileName, c)

        return ""

    def _hsv_mask(self, hsv,
                  hue_min, hue_max,
                  sat_min, sat_max,
                  val_min, val_max):
        lower = np.array([hue_min, sat_min, val_min])
        upper = np.array([hue_max, sat_max, val_max])
        return cv2.inRange(hsv, lower, upper)

    def _contours_in_hue_range(self, hsv, hue_range):
        hue_min = hue_range[0]
        hue_max = hue_range[1]
        sat_min = hue_range[2]
        sat_max = hue_range[3]
        val_min = hue_range[4]
        val_max = hue_range[5]
        if hue_max > hue_min:
            mask = self._hsv_mask(hsv,
                                  hue_min, hue_max,
                                  sat_min, sat_max,
                                  val_min, val_max)
        else:
            mask = (self._hsv_mask(hsv,
                                   hue_min, 180,
                                   sat_min, sat_max,
                                   val_min, val_max) +
                    self._hsv_mask(hsv,
                                   0, hue_max,
                                   sat_min, sat_max,
                                   val_min, val_max))

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
        for blinkers in range(70, 0, -10):
            result = self._white_line(file_name, blinkers)
            if result != "":
                return result
        return ""

    def do_test_white_line(self):
        for blinkers in range(70, 0, -10):
            print("Result with %d%% blinkers" % blinkers,
                  self._white_line("test-white-line.jpg", blinkers))
        return ""

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
        variances = []
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
                # Now calculate the variance.
                mean = moment / count
                variance = 0
                for i in range(blinkers, 200 - blinkers):
                    y = yrow[i-blinkers]
                    if y > yavg:
                        variance += (i - mean) * (i - mean) * ynorm
                variances.append(variance)
                if variance < 1000 and variance > 10:
                    centres.append(moment / count)
                    jvalues.append(j)
                    ymins.append(ymin)
                    ymaxs.append(ymax)
        print(centres)
        print(jvalues)
        print(ymins)
        print(ymaxs)
        print("Variances", variances)

        if len(centres) < 12:
            return ""

        # Fit a straight line to those centres.
        fit = np.polyfit(np.array(jvalues),
                         np.array(centres),
                         1)
        print(fit)

        return "%f %f" % (fit[0], fit[1])


cs = CommandServer()
cs.start()
cs.serve_commands()
