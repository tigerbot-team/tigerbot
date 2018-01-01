{{
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// METABOT3 Motor Controller
//
// Serial interface, see help messages below
//
// I2C interface : registers
//      0 :     Target Motor 0 Speed 
//      1 :     Target Motor 0 Direction
//      2 -7 :  Speed and Direction for Motors 1-3
//      8 :     AutoPing Rate (0 = off, 1 = continuous, other = not implemented)
//      9 :     Left Ping distance (x3 for distance in millimetres)
//      10:     Right Ping distance (ditto)
//      11:     Front Ping distance (ditto)
// 
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
}}

CON
  _CLKMODE = xtal1 + pll16x
  _XINFREQ = 6_000_000
  debuglim = 8

OBJ
  ps : "propshell"
  quad :  "encoder"
  pwm : "pibal_pwm"
  i2c : "I2C_slave"
  ping : "Ping"

DAT
'       M1   M2   M3   M4
' encA  A1   A10  A15  A29
' encB  A0   A11  A16  A28
' dir   A3   A12  A18  A26
' pwm   A2   A13  A17  A27

' Motor driver connector: DIR, PWM, GND
' Motor connector: encA, encB, +ve, gnd, M+, M-

  encoderPins byte 1, 0, 10, 11, 15, 16, 29, 28
  
  motorPWM    byte 2, 13, 17, 27
  motorD1     byte 3, 12, 18, 26

' Pinger pins  
  trigPins      byte 4, 6, 8
  echoPins      byte 5, 7, 9

VAR
  long  pidstack[30]
  long  pingstack[30]
  long  lastpos[4]
  long  debug[debuglim]
  long  actual_speed[4]
  long  error_integral[4]
  long  error_derivative[4]
  long  millidiv
  long  millioffset
  long Kp
  long Ki
  long Kd
  
PUB main
  millidiv := clkfreq / 1000
  Kp := 20
  Ki := 2
  Kd := 10
  millioffset := negx / millidiv * -1
  ps.init(string(">"), string("?"), 115200, 31, 30)   ' start the command interpreter shell (COG 1)
  i2c.start(28,29,$42)                                ' (COG 2)
  quad.Start(@encoderPins)                            ' start the quadrature encoder reader (COG 3)
  resetMotors                                         ' reset the motors
  pwm.start_pwm(motorPWM[0], motorPWM[1], motorPWM[2], motorPWM[3], 20000)    ' start the pwm driver (COGS 4 & 5)
  'cognew(pid, @pidstack)
  cognew(nopid,@pidstack)                             ' COG 6
  cognew(autoping, @pingstack)                        ' COG 7 (last one)
  
  ' Very weird if I add one more line to main it causes the ps prompt to get corrupted
  ' Line below is least necessary so commenting out
  ps.puts(string("Propeller starting...", ps#CR))

  repeat
    result := ps.prompt
    \cmdHandler(result)

PRI cmdHandler(cmdLine)
  cmdSetSpeed(ps.commandDef(string("+ss"), string("Set speed <motor[0..3], speed[-100...100]>") , cmdLine))
  cmdSetSpeedAll(ps.commandDef(string("+sa"), string("Set all speeds <speed[-100...100] x 4>") , cmdLine))
  cmdSetStop(ps.commandDef(string("+st"), string("Stop") , cmdLine))
  cmdSetPID(ps.commandDef(string("+sp"), string("Set PID Parameters <Kp Ki Kd>") , cmdLine))
  cmdGetPos(ps.commandDef(string("+gp"), string("Get position") , cmdLine))
  cmdGetSpeed(ps.commandDef(string("+gs"), string("Get speed") , cmdLine))
  cmdGetTime(ps.commandDef(string("+gt"), string("Get time") , cmdLine))
  cmdPing(ps.commandDef(string("+p"), string("Ping") , cmdLine))
  cmdGetDebug(ps.commandDef(string("+gd"), string("Get debug") , cmdLine))
  ps.puts(string("? for help", ps#CR))  ' no command recognised
  return true

PRI cmdSetSpeedAll(forMe) | motor, newspeed
  if not forMe
    return
  repeat motor from 0 to 3
    ps.parseAndCheck(motor+1, string("!ERR 1"), true)
    newspeed := ps.currentParDec
    ps.putd(newspeed)
    ps.puts(string(" "))
    i2c.put(motor*2, ||newspeed)
    if newspeed > 0
      i2c.put(motor*2+1, 0)
    else
      i2c.put(motor*2+1, 1)
  ps.puts(string(ps#CR))
  ps.commandHandled
  
PRI cmdSetSpeed(forMe) | motor, newspeed
  if not forMe
    return
  ps.parseAndCheck(1, string("!ERR 1"), true)
  motor := ps.currentParDec
  if motor < 0 or motor > 3
    ps.puts(string("!ERR 2", ps#CR))
    abort
  ps.parseAndCheck(2, string("!ERR 3"), true)
  newspeed := ps.currentParDec
  ps.puts(string("Set Motor Speed "))
  ps.putd(motor)
  ps.puts(string(", "))
  ps.putd(newspeed)
  ps.puts(string(ps#CR))
  i2c.put(motor*2, ||newspeed)
  if newspeed > 0
    i2c.put(motor*2+1, 0)
  else
    i2c.put(motor*2+1, 1)
  ps.commandHandled

PRI cmdSetStop(forMe)
  if not forMe
    return
  resetMotors
  ps.puts(string("Stopped"))
  ps.puts(string(ps#CR))
  ps.commandHandled

PRI cmdSetPID(forMe) | motor, newKp, newKi, newKd
  if not forMe
    return
  ps.parseAndCheck(1, string("!ERR 1"), true)
  newKp := ps.currentParDec
  ps.parseAndCheck(2, string("!ERR 2"), true)
  newKi := ps.currentParDec
  ps.parseAndCheck(3, string("!ERR 3"), true)
  newKd := ps.currentParDec

  ps.puts(string("Set PID Parameters "))
  ps.putd(newKp)
  ps.puts(string(", "))
  ps.putd(newKi)
  ps.puts(string(", "))
  ps.putd(newKd)
  ps.puts(string(ps#CR))
  Kp := newKp
  Ki := newKi
  Kd := newKd
  ps.commandHandled
  
PRI cmdGetPos(forMe) | i
  if not forMe
    return
  repeat i from 0 to 3
    ps.putd(lastpos[i])
    ps.puts(string(" "))
  ps.putd(millioffset + cnt/millidiv)  
  ps.puts(string(ps#CR))
  ps.commandHandled
  
PRI cmdGetSpeed(forMe) | i
  if not forMe
    return
  repeat i from 0 to 3
    ps.putd(actual_speed[i])
    ps.puts(string(" "))
  ps.putd(millioffset + cnt/millidiv)
  ps.puts(string(ps#CR))
  ps.commandHandled

PRI cmdGetTime(forMe)
  if not forMe
    return
  ps.putd(millioffset + cnt/millidiv)
  ps.puts(string(ps#CR))
  ps.commandHandled
  
PRI cmdGetDebug(forMe) | i
  if not forMe
    return
  ps.puts(string(ps#CR))
  repeat i from 0 to debuglim-1
    ps.putd(i)
    ps.puts(string(": "))
    ps.putd(debug[i])
    ps.puts(string(ps#CR))
  ps.commandHandled

PRI cmdPing(forMe) | i
  if not forMe
    return
  if i2c.get(8) == 0         ' if autopinger not running do one manually
    doPing(0)
    doPing(1)
  ps.puts(string("Ping: "))
  ps.putd(i2c.get(9) * 3)
  ps.puts(string(" mm, "))
  ps.putd(i2c.get(10) * 3)
  ps.puts(string(" mm"))
  ps.puts(string(ps#CR))
  ps.commandHandled
 
PRI doPing(side) | m
  side := 0 #> side <# 1
  m := ping.Millimetres(trigPins[side], echoPins[side]) / 3
  m <#= 255
  i2c.put(side+9, m)
 
PRI autoPing | nexttime
  i2c.put(8, 0)                                       ' auto-pinger turned off
  i2c.put(9, 0)                                       ' zero ping results
  i2c.put(10, 0)                                      
  i2c.put(11, 0)                                      
  nexttime := millidiv + cnt
  repeat
    waitcnt(nexttime)
    nexttime += millidiv * 100
    if i2c.get(8) > 0                   ' autopinger is enabled
      i2c.put(9, i2c.get(9) + 1)        ' test !
      i2c.put(10, i2c.get(10) - 1)      ' test
 
PRI resetMotors | i
  repeat i from 0 to 3
    i2c.put(i*2,0)
    i2c.put(i*2+1,0)
    error_integral[i] := 0
    outa[motorPWM[i]] := %0
    outa[motorD1[i]] := %0
'   outa[motorD2[i]] := %0
    dira[motorPWM[i]] := %1
    dira[motorD1[i]] := %1
'   dira[motorD2[i]] := %1
{{
PRI pid | i, nextpos, error, last_error, nexttime, newspeed, desired_speed
  nextpos := 0
  resetMotors    ' enables the direction ports control from this cog
  nexttime := millidiv + cnt
  repeat
    waitcnt(nexttime)
    nexttime += millidiv * 2
    'Here once every 2 milliseconds
   
    repeat i from 0 to 3          ' loop takes just under 1ms to complete
      desired_speed := i2c.get(i*2)
      if i2c.get(i*2+1) > 0
        desired_speed := -desired_speed
      debug[i] := desired_speed  
      nextpos := quad.count(i)
      last_error := desired_speed - actual_speed[i] 
      actual_speed[i] := nextpos - lastpos[i]
      lastpos[i] := nextpos
      error := desired_speed - actual_speed[i] 
      error_derivative[i] := error - last_error
      error_integral[i] += error
      newspeed := Kp * error + Ki * error_integral[i] + Kd * error_derivative[i]
      setMotorSpeed(i, newspeed)
}}
PRI nopid | i, nextpos, error, last_error, nexttime, newspeed, desired_speed
  nextpos := 0
  resetMotors    ' enables the direction ports control from this cog
  nexttime := millidiv + cnt
  repeat
    waitcnt(nexttime)
    nexttime += millidiv * 2
    'Here once every 2 milliseconds
   
    repeat i from 0 to 3          ' loop takes just under 1ms to complete
      lastpos[i] := quad.count(i)
      desired_speed := i2c.get(i*2)
      if i2c.get(i*2+1) > 0
        desired_speed := -desired_speed
      setMotorSpeed(i, desired_speed*10)
      
PRI setMotorSpeed(motor, speed)
  pwm.set_duty(motor, speed)
  if speed == 0
    outa[motorD1[motor]] := %0
'   outa[motorD2[motor]] := %0
  elseif speed > 0
    outa[motorD1[motor]] := %0
'   outa[motorD2[motor]] := %1
  else
    outa[motorD1[motor]] := %1
'   outa[motorD2[motor]] := %0
  
  
  
