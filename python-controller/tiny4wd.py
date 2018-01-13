# Code for Brian Corteil's Tiny 4WD robot, based on code from Brian as modified by Emma Norling.
# Subsequently modified by Tom Oinn to add dummy functions when no explorer hat is available,
# use any available joystick, use the new function in 1.0.6 of approxeng.input to get multiple
# axis values in a single call, use implicit de-structuring of tuples to reduce verbosity, add
# an exception to break out of the control loop on pressing HOME etc.

from time import sleep

import serial
import smbus
# All we need, as we don't care which controller we bind to, is the ControllerResource
from approxeng.input.selectbinder import ControllerResource
import approxeng
import logging
import traceback

approxeng.input.logger.setLevel(logging.INFO)

# From mb3.spin:
# I2C interface : registers
#      0 :     Motor 0 position (4 bytes)
#      4 :     Motor 1 position (4 bytes)
#      8 :     Motor 2 position (4 bytes)
#      12 :    Motor 3 position (4 bytes)
#      16 :    Left Ping distance in mm (2 bytes)
#      18 :    Right Ping distance in mm (2 bytes)
#      20 :    Front Ping distance in mm (2 bytes)
#      22 :    Target Motor 0 Speed (1 byte) signed -127 - 127
#      23 :    Target Motor 1 Speed (1 byte)
#      24 :    Target Motor 2 Speed (1 byte)
#      25 :    Target Motor 3 Speed (1 byte)
#      26 :    Options (low bit = autoping on / off)
#      27, 28 : Ball Thrower Motor Speed
#      29 :    Ball Thrower Servo 1
#      30 :    Ball Thrower Servo 2
#      31 :    Read Ready (master sets to 1 when ready to read,
#               slave sets to zero when multi-byte values updated

I2C_PORT = smbus.SMBus(1)
DEVICE_REG_MODE1 = 0x00

PROP_ADDR = 0x42
MOTOR1_REG = 22
MOTOR2_REG = 23
MOTOR3_REG = 24
MOTOR4_REG = 25
AUTO_PING_REG = 26
THROWER_SPD1_REG = 27
THROWER_SPD2_REG = 28
THROWER_SRV1_REG = 29
THROWER_SRV2_REG = 30
READ_RDY_REG = 31

try:
    # Attempt to import the Explorer HAT library. If this fails, because we're running somewhere
    # that doesn't have the library, we create dummy functions for set_speeds and stop_motors which
    # just print out what they'd have done. This is a fairly common way to deal with hardware that
    # may or may not exist! Obviously if you're actually running this on one of Brian's robots you
    # should have the Explorer HAT libraries installed, this is really just so I can test on my big
    # linux desktop machine when coding.

    from explorerhat import motor

    print('Explorer HAT library available.')


    def set_speeds(power_left, power_right):
        """
        As we have an motor hat, we can use the motors

        :param power_left:
            Power to send to left motor
        :param power_right:
            Power to send to right motor, will be inverted to reflect chassis layout
        """
        motor.one.speed(-power_right)
        motor.two.speed(power_left)


    def stop_motors():
        """
        As we have an motor hat, stop the motors using their motors call
        """
        motor.stop()

except ImportError:

    print('No explorer HAT library available, using dummy functions.')


    def set_speeds(power_left, power_right):
        """
        No motor hat - print what we would have sent to it if we'd had one.
        """
        print('Left: {}, Right: {}'.format(power_left, power_right))

        # Assemble a list of values for motor registers
        motor_values = [
            -power_left,
            -power_left,
            power_right,
            power_right,
        ]
        print "sending: %s" % motor_values
        i2c_block_send(motor_values)
        sleep(0.1)
        data = read_sensors()
        print "Read back: %s" % data

    def read_sensors():
        # Write 1 to Read Ready
        I2C_PORT.write_byte_data(PROP_ADDR, READ_RDY_REG, 1)
        # Wait for Read Ready to get set to 0
        while I2C_PORT.read_byte_data(PROP_ADDR, READ_RDY_REG) != 0:
            sleep(0.1)
        # Read all 32 registers
        data = I2C_PORT.read_i2c_block_data(PROP_ADDR, 0, 32)
        return data

    def i2c_block_send(data):
        I2C_PORT.write_i2c_block_data(PROP_ADDR, MOTOR1_REG, data)


    def stop_motors():
        """
        No motor hat, so just print a message.
        """
        print('Motors stopping')
        set_speeds(0, 0)


class RobotStopException(Exception):
    """
    The simplest possible subclass of Exception, we'll raise this if we want to stop the robot
    for any reason. Creating a custom exception like this makes the code more readable later.
    """
    pass


def mixer(yaw, throttle, expo=2.0, max_power=127):
    """
    Mix a pair of joystick axes, returning a pair of wheel speeds. This is where the mapping from
    joystick positions to wheel powers is defined, so any changes to how the robot drives should
    be made here, everything else is really just plumbing.

    :param yaw:
        Yaw axis value, ranges from -1.0 to 1.0
    :param throttle:
        Throttle axis value, ranges from -1.0 to 1.0
    :param expo:
        Factor which makes control less sensitive near centre, while
        still maintaining full control authority at full deflection
        Values less than 1 make the robot more 'twitchy'
        Values great than 1 make the robot less 'twitchy'
    :param max_power:
        Maximum speed that should be returned from the mixer, defaults to 127
    :return:
        A pair of power_left, power_right integer values to send to the motor driver
    """
    # Expo example: T = <output_range> * (I / <input_range>) ^ 3.3219
    # We do the sign and abs things to ensure we get a sane behaviour for negative inputs

    throttle_exp = sign(throttle) * max_power * ((abs(throttle) / 1) ** expo)
    yaw_exp = sign(yaw) * max_power * ((abs(yaw) / 1) ** expo)
    left = throttle_exp + yaw_exp
    right = throttle_exp - yaw_exp
    logging.info("mixer output: left: %s right:%s", int(left), int(right))
    return int(left), int(right)


def sign(data):
    if data >= 0:
        ret_val = 1
    else:
        ret_val = -1
    return ret_val


# Outer try / except catches the RobotStopException we just defined, which we'll raise when we want
# to bail out of the loop cleanly, shutting the motors down.
# We can raise this in response to a button press
try:
    while True:
        # Inner try / except is used to wait for a controller to become available, at which point we
        # bind to it and enter a loop where we read axis values and send commands to the motors.
        try:
            # Bind to any available joystick, this will use whatever's connected as long as the
            # library supports it.
            with ControllerResource(dead_zone=0.1, hot_zone=0.2) as joystick:
                print('Controller found, press HOME button to exit, use left stick to drive.')
                print(joystick.controls)
                # Loop until the joystick disconnects, or we deliberately stop by raising a
                # RobotStopException
                while joystick.connected:
                    # Get joystick values from the left analogue stick
                    x_axis, y_axis = joystick['lx', 'ly']
                    # Get power from mixer function
                    power_left, power_right = mixer(yaw=x_axis, throttle=y_axis)
                    # Set motor speeds
                    set_speeds(power_left, power_right)
                    # Get a ButtonPresses object containing everything that was pressed since the
                    # last time around this loop.
                    joystick.check_presses()
                    # Print out any buttons that were pressed, if we had any
                    if joystick.has_presses:
                        print(joystick.presses)
                    # If home was pressed, raise a RobotStopException to bail out of the loop
                    # Home is generally the PS button for playstation controllers, XBox for XBox etc
                    if 'home' in joystick.presses:
                        raise RobotStopException()
        except IOError:
            # We get an IOError when using the ControllerResource if we don't have a controller yet,
            # so in this case we just wait a second and try again after printing a message.
            traceback.print_exc()
            print('No controller found yet')
            sleep(1)
except RobotStopException:
    # This exception will be raised when the home button is pressed, at which point we should
    # stop the motors.
    stop_motors()
