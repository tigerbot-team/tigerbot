import cv2 as cv
import cv2
import numpy as np
from matplotlib import pyplot as plt
from picamera2 import Picamera2

sf = 0.250
template = cv.imread("zombie.png")
template = cv2.cvtColor(template, cv2.COLOR_BGR2RGB)
assert template is not None, "file could not be read, check with os.path.exists()"
template = cv.resize(template, (0, 0), fx=sf, fy=sf)
_, w, h = template.shape[::-1]

picam2 = Picamera2()
picam2.configure(picam2.create_preview_configuration(main={"format": 'XRGB8888', "size": (1536, 864)}))
picam2.start()

method = cv.TM_SQDIFF

while True:
    img = picam2.capture_array()
    img = cv2.cvtColor(img, cv2.COLOR_RGBA2RGB)
    # Apply template Matching
    res = cv.matchTemplate(img, template, method)
    min_val, max_val, min_loc, max_loc = cv.minMaxLoc(res)

    # If the method is TM_SQDIFF or TM_SQDIFF_NORMED, take minimum
    if method in [cv.TM_SQDIFF, cv.TM_SQDIFF_NORMED]:
        top_left = min_loc
    else:
        top_left = max_loc
    bottom_right = (top_left[0] + w, top_left[1] + h)

    cv.rectangle(img, top_left, bottom_right, (255, 255, 0), 5)
    cv.rectangle(img, top_left, bottom_right, (255, 255, 0), 5)

    plt.subplot(121), plt.imshow(res, cmap="inferno")
    plt.title("Matching Result"), plt.xticks([]), plt.yticks([])
    plt.subplot(122), plt.imshow(img)
    plt.title("Detected Point"), plt.xticks([]), plt.yticks([])
#    plt.suptitle(meth)

    frame_128 = cv2.resize(img, (128,128))
    frame_128 = cv2.rotate(frame_128, cv2.ROTATE_90_CLOCKWISE)
    frame_red_blue = cv2.cvtColor(frame_128, cv2.COLOR_RGB2BGR)
    frame_rgb565 = cv2.cvtColor(frame_red_blue, cv2.COLOR_RGB2BGR565)


    plt.show()
    with open('/dev/fb0', 'rb+') as buf:
        buf.write(frame_rgb565)

    break

