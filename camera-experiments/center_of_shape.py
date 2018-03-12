# import the necessary packages
import argparse
import imutils
import cv2
import numpy as np
import math

# Looking for 4 balls: yellow, orange, blue and green.
#
# https://www.learnopencv.com/color-spaces-in-opencv-cpp-python/
# advises using HSV colour space - because that encodes colour
# entirely in the H value, in a way that is relatively independent of
# illumination - and that a sensible H range for looking for a
# particular colour is -/+ 5-7 (when the overall H range is 0..255).
#
# In that blog, yellow is found at H 20-30, and blue at H 98-112.
#
# On the other hand, by loading a ball photo into Inkscape and picking
# colours from each ball, we get:
#
# yellow = H 16-18
# orange = H 254-0
# blue   = H 162-169
# green  = H 77-85
#
# (This is with Inkscape using the HLS colour space, but the H values
# in HSV and HLS are the same for a given colour.  Also Inkscape is
# also using 0..255 for each colour component.)
#
# Clearly there is a range of possible blues and yellows, and probably
# that's true for all the colours.  We don't know exactly which hue
# each coloured blue will appear as in the photo, but I think we can
# expect that that hue will be consistent across the whole ball shape.
#
# So, the kind of algorithm that I think would work is one that, for a
# given target colour:
#
# - starts from a wide H range, that we consider to be the complete
#   plausible range for that colour
#
# - loops through that range with a narrower (-/+ 5-7) H sliding
#   window
#
# - for each sliding window position:
#
#   - calculates a mask 0/1 image, in which each pixel is 1 if the H
#     at that position was within the sliding window, 0 otherwise
#
#   - convolutes that image with a kernel whose values are (1 - r/R),
#     for r = 0..R, where r is the distance of each kernel pixel from
#     the centre of the kernel and R is twice the radius that we guess
#     each ball will have in the image
#
#   - finds the pixel in the convolution that has the largest value,
#     and returns its position as the most likely ball position, for
#     that sliding window
#
# - returns the most likely ball position for the sliding window that
#   generated the largest (individual pixel) convolution value.

# Our test image is 922x100 pixels, and the balls are actually at:
#
# yellow: x=60
# orange: x=95
# blue: x=140
# green: x=180
#
# And they have a radius of about 20 pixels

# construct the argument parse and parse the arguments
ap = argparse.ArgumentParser()
ap.add_argument("-i", "--image", required=True,
	help="path to the input image")
args = vars(ap.parse_args())

# Load the image and convert it to HSV.
image = cv2.imread(args["image"])
cv2.imshow("Original", image)
hsv = cv2.cvtColor(image, cv2.COLOR_BGR2HSV)

# Plausible hue ranges.
yellow = [ 10, 40, "yellow", 60 ]
green = [ 60, 90, "green", 95 ]
blue = [ 95, 170, "blue", 140 ]
orange = [ 220, 260, "orange", 180 ]

# The H amount by which we move the sliding window for each calculation.
h_step = 4

# Twice the radius that we guess each ball to have (in pixels)
R = 30

def build_kernel():
    kernel = []
    ksum = 0
    y = -R
    while y <= R:
        row = []
        x = -R
        while x <= R:
            r = math.sqrt(x*x + y*y)
            if r < R:
                value = 1 - r/R
                row.append(value)
                ksum += value
            else:
                row.append(0)
            x += 1
        kernel.append(row)
        y += 1
    # Now normalize by dividing by ksum.
    return [[v/ksum for v in row] for row in kernel]

kernel = np.array(build_kernel())

def find_ball(colour):
    print "Looking for %r" % colour
    h_lo = colour[0]
    h_hi = h_lo + 14
    highest_value = 0
    best_x = best_y = None
    while h_hi <= colour[1]:
        x, y, value, mask = find_ball_in_h_range(h_lo % 256, h_hi % 256)
        print "H=%r-%r X=%r Y=%r VALUE=%r" % (h_lo,
                                              h_hi,
                                              x,
                                              y,
                                              value)
        if value > highest_value:
            best_x = x
            best_y = y
            highest_value = value
            cv2.imshow("Image", mask)
            cv2.waitKey(0)
            masked = cv2.cvtColor(cv2.bitwise_and(hsv, hsv, mask=mask),
                                  cv2.COLOR_HSV2BGR)
            cv2.imshow("Image", masked)
            cv2.waitKey(0)

        h_lo += h_step
        h_hi += h_step
    return best_x, best_y, highest_value

def find_ball_in_h_range(h_lo, h_hi):
    if h_hi > h_lo:
        mask = hue_mask(hsv, h_lo, h_hi)
    else:
        mask1 = hue_mask(hsv, h_lo, 256)
        mask2 = hue_mask(hsv, 0, h_hi)
        mask = cv2.bitwise_or(mask1, mask2)

    convolution = cv2.filter2D(mask, -1, kernel)
    (minVal, maxVal, minLoc, maxLoc) = cv2.minMaxLoc(convolution)
    return maxLoc[0], maxLoc[1], maxVal, mask

def hue_mask(hsv, h_lo, h_hi):
    return cv2.inRange(hsv,
                       np.array([int(h_lo*180/255), 60, 60]),
                       np.array([int(h_hi*180/255), 255, 255]))

cv2.imshow("Image", hsv)
cv2.waitKey(0)

find_ball(yellow)
find_ball(green)
find_ball(blue)
find_ball(orange)
