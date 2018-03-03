# import the necessary packages
import argparse
import imutils
import cv2
import numpy as np

# construct the argument parse and parse the arguments
ap = argparse.ArgumentParser()
ap.add_argument("-i", "--image", required=True,
	help="path to the input image")
args = vars(ap.parse_args())

# load the image, convert it to grayscale, blur it slightly,
# and threshold it
image = cv2.imread(args["image"])
hsv = cv2.cvtColor(image, cv2.COLOR_BGR2HLS)


#blurred = cv2.GaussianBlur(gray, (5, 5), 0)
#thresh = cv2.threshold(blurred, 60, 255, cv2.THRESH_BINARY)[1]

# According to Inkscape (HSL):
# yellow = H 16-18, S 90
# orange = H 254-0, S 90
# blue   = H 162-169, S 90
# green  = H 77-85, S 30
colors = [ [16, 18, 90], [162, 169, 90], [77, 85, 30], [254, 0, 90] ]
#colors = [ [10, 24], [156, 175], [71, 91], [248, 6] ]
#colors = [ [7, 27], [155, 175], [71, 91], [254, 0] ]

def hue_mask(hsv, h_lo, h_hi, s):
    h_lo -= 5
    h_hi += 5
    return cv2.inRange(hsv,
                       np.array([int(h_lo*180/255), 50, (s-30)*180/255]),
                       np.array([int(h_hi*180/255), 255, (s+30)*180/255]))

# display the image on screen and wait for a keypress
for x in colors:
    print x
    if x[1] > x[0]:
        mask = hue_mask(hsv, x[0], x[1], x[2])
    else:
        mask1 = hue_mask(hsv, x[0], 181, x[2])
        mask2 = hue_mask(hsv, 0, x[1], x[2])
        mask = cv2.bitwise_or(mask1, mask2)

    cv2.imshow("Image", mask)
    cv2.waitKey(0)
    cv2.imshow("Image", image)
    cv2.waitKey(0)
