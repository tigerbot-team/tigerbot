import time

import gobject
from explorerhat import motor

from joy import Joystick


class Robot:
    def __init__(self):
        # Setup
        self.x_axis = 0.0
        self.y_axis = 0.0
        self.max_power = 1.0
        self.disable_motor = True

    def mixer(self, in_yaw, in_throttle):
        left = in_throttle + in_yaw
        right = in_throttle - in_yaw
        scale_left = abs(left / 125.0)
        scale_right = abs(right / 125.0)
        scale_max = max(scale_left, scale_right)
        scale_max = max(1, scale_max)
        out_left = int(self.constrain(left / scale_max, -125, 125))
        out_right = int(self.constrain(right / scale_max, -125, 125))
        results = [out_right, out_left]
        return results

    @staticmethod
    def constrain(val, min_val, max_val):
        return min(max_val, max(min_val, val))

    def update_motor(self):
        mixer_results = self.mixer(self.x_axis, self.y_axis)
        # print (mixer_results)
        power_left = int((mixer_results[0] / 125.0) * 100)
        power_right = int((mixer_results[1] / 125.0) * 100)
        print("left: " + str(power_left) + " right: " + str(power_right))

        if not self.disable_motor:
            motor.one.speed((-power_right * self.max_power))
            motor.two.speed(power_left * self.max_power)

    def axis_handler(self, signal, number, value, init):
        # Axis 0 = left stick horizontal.  -ve = left
        # Axis 1 = left stick vertical.    -ve = up
        # Axis 5 = right stick vertical.   -ve = up
        # Axis 2 = right stick horizontal. -ve left
        if number == 5:
            if value > 130:
                print("Backwards")
            elif value < 125:
                print("Forward")
            self.y_axis = value
            if self.y_axis > 130:
                self.y_axis = -(self.y_axis - 130)
            elif self.y_axis < 125:
                self.y_axis = ((-self.y_axis) + 125)
            else:
                self.y_axis = 0.0
        elif number == 2:
            if value > 130:
                print("Right")
            elif value < 125:
                print("Left")
            self.x_axis = value
            if self.x_axis > 130:
                self.x_axis = (self.x_axis - 130)
            elif self.x_axis < 125:
                self.x_axis = -((-self.x_axis) + 125)
            else:
                self.x_axis = 0.0
            print("X: " + str(self.x_axis))
        self.update_motor()

    def run(self):
        try:
            j = Joystick(0)
            j.connect('axis', self.axis_handler)
            loop = gobject.MainLoop()
            context = loop.get_context()
            while True:
                if context.pending():
                    context.iteration(True)
                else:
                    time.sleep(0.01)
        except Exception, e:
            print(e)
            print("stop")
            motor.stop()
        print("bye")


if __name__ == "__main__":
    robot = Robot()
    robot.run()
