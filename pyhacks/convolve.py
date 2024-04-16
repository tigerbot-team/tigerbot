import cv2 
 
# path 
image = cv2.imread('./aim.jpg') 
small = cv2.resize(image, (0,0), fx=0.125, fy=0.125) 

kernel = cv2.imread('./zombie.png')

# Window name in which image is displayed 
window_name = 'image'
  
# Using cv2.imshow() method 
# Displaying the image 
cv2.imshow(window_name, small) 
  
# waits for user to press any key 
# (this is necessary to avoid Python kernel form crashing) 
cv2.waitKey(0) 
  
# closing all open windows 
cv2.destroyAllWindows() 
