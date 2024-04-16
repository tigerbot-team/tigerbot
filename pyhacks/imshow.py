import cv2 
 
# path 
image = cv2.imread('./aim.jpg') 
small = cv2.resize(image, (0,0), fx=0.125, fy=0.125) 

# Window name in which image is displayed 
window_name = 'image'

frame_128 = cv2.resize(image, (128,128))
frame_128 = cv2.rotate(frame_128, cv2.ROTATE_90_CLOCKWISE)
frame_red_blue = cv2.cvtColor(frame_128, cv2.COLOR_RGB2BGR)
frame_rgb565 = cv2.cvtColor(frame_red_blue, cv2.COLOR_RGB2BGR565)

with open('/dev/fb0', 'rb+') as buf:
    buf.write(frame_rgb565)
  
# Using cv2.imshow() method 
# Displaying the image 
cv2.imshow(window_name, small) 
  
# waits for user to press any key 
# (this is necessary to avoid Python kernel form crashing) 
cv2.waitKey(0) 
  
# closing all open windows 
cv2.destroyAllWindows() 
