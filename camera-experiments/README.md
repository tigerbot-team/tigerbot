
* Setup for running Python/OpenCV with camera on Pi

From a machine with an X server:

	ssh -X pi@<IP of Pi>

`-X` allows the Pi to run X applications (like OpenCV), with their windows
appearing on your host machine.

Then on the Pi, clone this repo, cd into it, and

	make run-container

Then in the container you can run Python code that accesses the camera and uses
OpenCV, imutils, numpy etc.  For example, to capture and show a single image:

	python test-image.py

(Code taken from
https://www.pyimagesearch.com/2015/03/30/accessing-the-raspberry-pi-camera-with-opencv-and-python/.)
