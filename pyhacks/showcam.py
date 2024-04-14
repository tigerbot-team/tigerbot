import cv2 
from picamera2 import Picamera2 
# path 

# Window name in which image is displayed 
window_name = 'image'

picam2 = Picamera2()
picam2.configure(picam2.create_preview_configuration(main={"format": 'XRGB8888', "size": (640, 480)}))
picam2.start()

while True:
    im = picam2.capture_array()
    #grey = cv2.cvtColor(im, cv2.COLOR_BGR2GRAY)
    cv2.rectangle(im, (320-10, 240), (320+30, 278), (255, 0, 0), 5)
    im = im[180:308,320-64:320+64]
    frame_128 = cv2.resize(im, (128,128))
    frame_128 = cv2.rotate(frame_128, cv2.ROTATE_90_CLOCKWISE)
    frame_red_blue = cv2.cvtColor(frame_128, cv2.COLOR_RGB2BGR)
    frame_rgb565 = cv2.cvtColor(frame_red_blue, cv2.COLOR_RGB2BGR565)

    with open('/dev/fb0', 'rb+') as buf:
        buf.write(frame_rgb565)
  
