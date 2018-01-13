from time import sleep
import smbus

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


def read_sensors():
    # Write 1 to Read Ready
    I2C_PORT.write_byte_data(PROP_ADDR, READ_RDY_REG, 1)
    # Wait for Read Ready to get set to 0
    while I2C_PORT.read_byte_data(PROP_ADDR, READ_RDY_REG) != 0:
        print "waiting"
        sleep(0.1)
    # Read all 32 registers
    data = I2C_PORT.read_i2c_block_data(PROP_ADDR, 0, 32)
    print data


read_sensors()
motor_data = [1, 1, 1, 1]
I2C_PORT.write_i2c_block_data(PROP_ADDR, MOTOR1_REG, motor_data)
read_sensors()
sleep(1)
motor_data = [0, 0, 0, 0]
I2C_PORT.write_i2c_block_data(PROP_ADDR, MOTOR1_REG, motor_data)
while True:
    read_sensors()
    sleep(1)
