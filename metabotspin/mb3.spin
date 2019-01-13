{{
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// METABOT3 Motor Controller
//
// I2C interface : registers
//      0 :     Motor 0 position (4 bytes)
//      4 :     Motor 1 position (4 bytes)
//      8 :     Motor 2 position (4 bytes)
//      12 :    Motor 3 position (4 bytes)
//      16 :    Left Ping distance in mm (2 bytes)
//      18 :    Right Ping distance in mm (2 bytes)
//      20 :    Front Ping distance in mm (2 bytes)
//      22 :    Target Motor 0 Speed (1 byte) signed -127 - 127
//      23 :    Target Motor 1 Speed (1 byte)
//      24 :    Target Motor 2 Speed (1 byte)
//      25 :    Target Motor 3 Speed (1 byte)
//      26 :    Options (low bit = autoping on / off)
//      27 :    Ball Thrower Motor Speed 1
//      28 :    Ball Thrower Motor Speed 2
//      29 :    Ball Thrower Servo 1
//      30 :    Ball Thrower Servo 2
//      31 :    Read Ready (master sets to 1 when ready to read, slave sets to zero when multi-byte values updated
// 
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
}}

CON
  _CLKMODE = xtal1 + pll16x
  _XINFREQ = 6_000_000
  debuglim = 8
  posbase = 0
  pingbase = 16
  speedbase = 22
  options = 26
  ballmotor1 = 27
  ballmotor2 = 28
  servo1 = 29
  servo2 = 30
  readready = 31
  motorshutdowntime = 100
  maxpwmramp = 100

OBJ
  quad :  "encoder"
  pwm : "pibal_pwm"
  i2c : "I2C_slave"
  ping : "Ping"

DAT
'       M1   M2   M3   M4
' encA  A1   A10  A15  A29
' encB  A0   A11  A14  A28
' dir   A3   A12  A18  A26
' pwm   A2   A13  A17  A27

' Motor driver connector: DIR, PWM, GND
' Motor connector: encA, encB, +ve, gnd, M+, M-

  encoderPins byte 0, 1, 11, 10, 14, 15, 28, 29
  motorPWM    byte 2, 13, 17, 27
  motorD1     byte 3, 12, 18, 26

' Pinger pins  
  trigPins      byte 4, 6, 8
  echoPins      byte 5, 7, 9

' Servo Pins - 19, 20, 21, (22) (22 is a copy of 21)

VAR
  long  pidstack[80]
  long  pingstack[30]
  word  pingval[3]
  long  lastpos[4]
  long  debug[debuglim]
  long  actual_speed[4]
  byte  time_at_zero[4]
  long  error_integral[4]
  long  error_derivative[4]
  long  millidiv
  long  servozero
  long  servofactor
  long  motorfactor
  long  millioffset
  long  Kp
  long  Ki
  long  Kd
  byte  b
  long  timeout
  long  lastping
  long  pingcog
  
  ' vars for servo program      
  long  position1, position2, position3, position4    'The assembly program will read these variables from the main Hub RAM to determine
                                           ' the high pulse durations of the three servo signals                 
  
PUB main
  millidiv := clkfreq / 1000

  servozero := millidiv * 80 / 100      ' Minimum servo pulse width.  This should be 1ms but we stretch it a little
                                        ' for slightly greater travel.
  servofactor := millidiv * 140 / 25600 ' Scale factor from servo units to pulse time.
  motorfactor := millidiv / 256         ' Scale factor from servo units to motor pulse time.

  Kp := 20
  Ki := 4
  Kd := 10
  timeout := 100
  lastping := cnt

  ' Since we pre-calculate the servo delays in this thread we need to set them before we start the servo control cog.
  position1 := millidiv
  position2 := millidiv
  position3 := servozero + (127 * servofactor)
  position4 := servozero + (127 * servofactor)

  p1:=@position1                           'Stores the address of the "position1" variable in the main Hub RAM as "p1"
  p2:=@position2                           'Stores the address of the "position2" variable in the main Hub RAM as "p2"
  p3:=@position3                           'Stores the address of the "position3" variable in the main Hub RAM as "p3"
  p4:=@position4                           'Stores the address of the "position4" variable in the main Hub RAM as "p3"

  waitcnt(millidiv + cnt)

  cognew(@ThreeServos,0)                   'Start a new cog and run the assembly code starting at the "ThreeServos" cell         

  millioffset := negx / millidiv * -1
  i2c.start(24,25,$42)                                ' (COG 2)  Start I2C using pins 24 CLK / 25 DAT at address 0x42
  quad.Start(@encoderPins)                            ' start the quadrature encoder reader (COG 3)
  resetMotors                                         ' reset the motors
  pwm.start_pwm(motorPWM[0], motorPWM[1], motorPWM[2], motorPWM[3], 20000)    ' start the pwm driver (COGS 4 & 5)
  cognew(pid, @pidstack)                              ' COG 6
  pingcog := cognew(autoping, @pingstack)              ' COG 7 (last one)
  repeat
    update
    waitcnt(millidiv + cnt)
    ' check pinger timeout and restart cog if it has fired
    'if (cnt - lastping)  >  ((clkfreq / millidiv) * 100)
    '  coginit(pingcog, autoping, @pingstack)

PRI update | i
  ' If host is ready to read then write ping and position values
  timeout := timeout + 1
  if timeout > 100 or i2c.get(readready) > 0
    timeout := 0
    repeat i from 0 to 2
      i2c.putw(pingbase+i*2, pingval[i])
    repeat i from 0 to 3  
      i2c.putl(posbase+i*4, lastpos[i])
    i2c.put(readready, 0)                   ' reset ready for the next read

PRI resetMotors | i
  repeat i from 0 to 3
    i2c.put(speedbase+i,0)
    error_integral[i] := 0
    outa[motorPWM[i]] := %0
    outa[motorD1[i]] := %0
    dira[motorPWM[i]] := %1
    dira[motorD1[i]] := %1
    
PRI doPing(side) | m
  side := 0 #> side <# 2
  pingval[side] := ping.Millimetres(trigPins[side], echoPins[side])
 
PRI autoPing | i
  i2c.put(options, 1)                                 ' auto-pinger turned on
  i2c.putw(pingbase, 0)                               ' zero ping results
  i2c.putw(pingbase+2, 0)                                
  i2c.putw(pingbase+4, 0)                                      
  repeat
    ' Do autoping
    lastping := cnt
    if i2c.get(options) > 0                   ' autopinger is enabled
      doPing(0)
      doPing(1)
    if i2c.get(options) == 2
      doPing(2)
    else
      i2c.putw(pingbase+4, 0)                                      
 
PRI pid | i, nextpos, error, last_error, nexttime, lastspeed[4], newspeed, desired_speed, maxintegral, servoval
  nextpos := 0
  maxintegral := 1000 / Ki
  resetMotors    ' enables the direction ports control from this cog
  nexttime := millidiv + cnt
  lastspeed[0] := 0
  lastspeed[1] := 0
  lastspeed[2] := 0
  lastspeed[3] := 0
  repeat
    waitcnt(nexttime)
    nexttime += millidiv * 5
    'Here once every 5 milliseconds
   
    ' Update motor speeds
    repeat i from 0 to 3          ' loop takes just under 1ms to complete
      b := i2c.get(speedbase+i)   
      desired_speed := ~b         ' note sneaky '~' that sign extends the byte value
      nextpos := quad.count(i)

      last_error := desired_speed - actual_speed[i] 
      actual_speed[i] := (nextpos - lastpos[i]) * 3
      lastpos[i] := nextpos

      if desired_speed == 0
        ' count how long we've been at zero speed
        time_at_zero[i] := time_at_zero[i] + 1 <# motorshutdowntime
      else
        time_at_zero[i] := 0

      if time_at_zero[i] < motorshutdowntime
        error := desired_speed - actual_speed[i]
        error_derivative[i] := error - last_error
        error_integral[i] := (error_integral[i] * 100 / 99) + error
        error_integral[i] := -maxintegral #> error_integral[i] <# maxintegral
        newspeed := Kp * error + Ki * error_integral[i] + Kd * error_derivative[i]
      else
        ' Desired speed has been at zero speed for some time, turn it off to avoid PID judder.
        error_integral[i] := 0
        newspeed := 0

      newspeed := (lastspeed[i] - maxpwmramp) #> newspeed <# (lastspeed[i] + maxpwmramp)  ' set a maximum PWM ramp
      setMotorSpeed(i, newspeed)
      lastspeed[i] := newspeed
      
    ' Update servo parameters  
    position1 := (i2c.get(ballmotor1) * motorfactor) + millidiv
    position2 := (i2c.get(ballmotor2) * motorfactor) + millidiv
    position3 := (i2c.get(servo1) * servofactor) + servozero
    position4 := (i2c.get(servo2) * servofactor) + servozero
      
PRI setMotorSpeed(motor, speed)
  pwm.set_duty(motor, speed)
  
  if speed == 0
    outa[motorD1[motor]] := %0
  elseif speed > 0
    outa[motorD1[motor]] := %0
  else
    outa[motorD1[motor]] := %1
 
DAT
'The assembly program below runs on a parallel cog and checks the value of the "position1", "position2" and "position3"
' variables in the main Hub RAM (which other cogs can change at any time). It then outputs three servo high pulses (back to
' back) each corresponding to the three position variables (which represent the number of system clock ticks during which
' each pulse is outputed) and sends a 10ms low part of the pulse. It repeats this signal continuously and changes the width
' of the high pulses as the "position1", "position2" and "position3" variables are changed by other cogs.

ThreeServos   org                         'Assembles the next command to the first cell (cell 0) in the new cog's RAM

              mov       loopcounter,cnt
              add       loopcounter,LoopTime

Loop          mov       dira,ServoPin1    'Set the direction of the "ServoPin1" to be an output (and all others to be inputs)  
              rdlong    HighTime,p1       'Read the "position1" variable from Main RAM and store it as "HighTime"
              mov       counter,cnt       'Store the current system clock count in the "counter" cell's address 
              mov       outa,AllOn        'Set all pins on this cog high (really only sets ServoPin1 high b/c rest are inputs)               
              add       counter,HighTime  'Add "HighTime" value to "counter" value
              waitcnt   counter,0         'Wait until cnt matches counter (adds 0 to "counter" afterwards)
              mov       outa,#0           'Set all pins on this cog low (really only sets ServoPin1 low b/c rest are inputs)

              mov       dira,ServoPin2    'Set the direction of the "ServoPin2" to be an output (and all others to be inputs)  
              rdlong    HighTime,p2       'Read the "position2" variable from Main RAM and store it as "HighTime"
              mov       counter,cnt       'Store the current system clock count in the "counter" cell's address 
              mov       outa,AllOn        'Set all pins on this cog high (really only sets ServoPin2 high b/c rest are inputs)               
              add       counter,HighTime  'Add "HighTime" value to "counter" value
              waitcnt   counter,0         'Wait until cnt matches counter (adds 0 to "counter" afterwards)
              mov       outa,#0           'Set all pins on this cog low (really only sets ServoPin2 low b/c rest are inputs)

              mov       dira,ServoPin3    'Set the direction of the "ServoPin3" to be an output (and all others to be inputs)
              rdlong    HighTime,p3       'Read the "position3" variable from Main RAM and store it as "HighTime"
              mov       counter,cnt       'Store the current system clock count in the "counter" cell's address
              mov       outa,AllOn        'Set all pins on this cog high (really only sets ServoPin3 high b/c rest are inputs)
              add       counter,HighTime  'Add "HighTime" value to "counter" value
              waitcnt   counter,0         'Wait until cnt matches counter (adds 0 to "counter" afterwards)
              mov       outa,#0           'Set all pins on this cog low (really only sets ServoPin3 low b/c rest are inputs)

              mov       dira,ServoPin4    'Set the direction of the "ServoPin2" to be an output (and all others to be inputs)
              rdlong    HighTime,p4       'Read the "position4" variable from Main RAM and store it as "HighTime"
              mov       counter,cnt       'Store the current system clock count in the "counter" cell's address
              mov       outa,AllOn        'Set all pins on this cog high (really only sets ServoPin4 high b/c rest are inputs)
              add       counter,HighTime  'Add "HighTime" value to "counter" value
              waitcnt   counter,0         'Wait until cnt matches counter (adds 0 to "counter" afterwards)
              mov       outa,#0           'Set all pins on this cog low (really only sets ServoPin4 low b/c rest are inputs)

              waitcnt   loopcounter,LoopTime   ' Wait for new loop (and auto-add LoopTime to the counter)
              jmp       #Loop             'Jump back up to the cell labled "Loop"                                      
                                                                                                                    
'Constants and Variables:
ServoPin1     long      |<      19 '<------- This sets the pin that outputs the first servo signal (which is sent to the white
                                          ' wire on most servomotors). Here, this "6" indicates Pin 6. Simply change the "6" 
                                          ' to another number to specify another pin (0-31).
ServoPin2     long      |<      20 '<------- This sets the pin that outputs the second servo signal (could be 0-31).
ServoPin3     long      |<      21 '<------- This sets the pin that outputs the third servo signal (could be 0-31).
ServoPin4     long      |<      22 '<------- This sets the pin that outputs the fourth servo signal.
p1            long      0                 'Used to store the address of the "position1" variable in the main RAM
p2            long      0                 'Used to store the address of the "position2" variable in the main RAM  
p3            long      0                 'Used to store the address of the "position3" variable in the main RAM
p4            long      0                 'Used to store the address of the "position4" variable in the main RAM
AllOn         long      $FFFFFFFF         'This will be used to set all of the pins high (this number is 32 ones in binary)
LoopTime      long      1_920_000         '20ms
counter       res                         'Reserve one long of cog RAM for this "counter" variable
loopcounter   res
HighTime      res                         'Reserve one long of cog RAM for this "HighTime" variable
              fit                         'Makes sure the preceding code fits within cells 0-495 of the cog's RAM

