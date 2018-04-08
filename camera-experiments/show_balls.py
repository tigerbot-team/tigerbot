# import the necessary packages
import argparse
import imutils
import cv2
import numpy as np
import math

# construct the argument parse and parse the arguments
ap = argparse.ArgumentParser()
ap.add_argument("-i", "--image", required=True,
	help="path to the input image")
args = vars(ap.parse_args())

# Load the image, resize for faster processing.
image = cv2.imread(args["image"])
image = imutils.resize(image, width=600)

# Convert to HSV.
hsv = cv2.cvtColor(image, cv2.COLOR_BGR2HSV)

# Ball colours to look for, and the HSV ranges that are most effective
# for picking out those colours.
ball_colours = {
    "yellow": [ 21, 42, 0, 255, 0, 255 ],
    "green": [ 60, 100, 60, 255, 0, 255 ],
    "blue": [ 100, 120, 90, 255, 80, 255 ],
    "orange": [ 165, 10, 120, 255, 60, 255 ],
}

def hue_mask(hsv, h_lo, h_hi, s_lo, s_hi, v_lo, v_hi):
    return cv2.inRange(hsv,
                       np.array([h_lo, s_lo, v_lo]),
                       np.array([h_hi, s_hi, v_hi]))

def hsv_mask(hsv, h_lo, h_hi, s_lo, s_hi, v_lo, v_hi):
    if h_hi > h_lo:
        return hue_mask(hsv, h_lo, h_hi, s_lo, s_hi, v_lo, v_hi)
    else:
        mask1 = hue_mask(hsv, h_lo, 180, s_lo, s_hi, v_lo, v_hi)
        mask2 = hue_mask(hsv, 0, h_hi, s_lo, s_hi, v_lo, v_hi)
        return cv2.bitwise_or(mask1, mask2)

for colour, hsv_range in ball_colours.iteritems():
    print "Looking for %r" % colour
    h_lo = hsv_range[0]
    h_hi = hsv_range[1]
    s_lo = hsv_range[2]
    s_hi = hsv_range[3]
    v_lo = hsv_range[4]
    v_hi = hsv_range[5]
    while True:
        mask = hsv_mask(hsv, h_lo, h_hi, s_lo, s_hi, v_lo, v_hi)
	mask = cv2.erode(mask, None, iterations=2)
	mask = cv2.dilate(mask, None, iterations=2)

	# find contours in the mask and initialize the current
	# (x, y) center of the ball
	cnts = cv2.findContours(mask.copy(), cv2.RETR_EXTERNAL,
		cv2.CHAIN_APPROX_SIMPLE)[-2]
	center = None

	# only proceed if at least one contour was found
	if len(cnts) > 0:
		# find the largest contour in the mask, then use
		# it to compute the minimum enclosing circle and
		# centroid
		c = max(cnts, key=cv2.contourArea)
		((x, y), radius) = cv2.minEnclosingCircle(c)
		M = cv2.moments(c)
		center = (int(M["m10"] / M["m00"]), int(M["m01"] / M["m00"]))

		# only proceed if the radius meets a minimum size
		if radius > 10:
			# draw the circle and centroid on the frame,
			# then update the list of tracked points
			cv2.circle(image, (int(x), int(y)), int(radius),
				(0, 255, 255), 2)
			cv2.circle(image, center, 5, (0, 0, 255), -1)
        break

cv2.imshow("Showing balls", image)
cv2.waitKey(0)
