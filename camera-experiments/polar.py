import math
import cv2
import numpy as np

img = cv2.imread("last.jpg")

w = img.shape[1]
h = 100

map_xf = np.zeros((h,w),np.float32)
map_yf = np.zeros((h,w),np.float32)

for x in xrange(w):
    for y in xrange(h):
        theta = x * math.pi * 2 / w
        r = 120.0 + y*0.9
        map_xf.itemset((y,x), 474 + math.cos(theta) * r)
        map_yf.itemset((y,x), 494 + math.sin(theta) * r)

map_x = np.zeros((h,w),np.uint16)
map_y = np.zeros((h,w),np.uint16)

map_x, map_y = cv2.convertMaps(map_xf, map_yf, cv2.CV_16SC2)

import time
start = time.time()
out = cv2.remap(img, map_x, map_y, cv2.INTER_LINEAR)
end = time.time()
print end-start

cv2.imwrite("unwarped.jpg", out)

cv2.imshow('dst',out)
if cv2.waitKey(100000)==27:
    pass
