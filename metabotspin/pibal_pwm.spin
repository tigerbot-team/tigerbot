{{
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// METABOT3 PWM Controller
//
///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
}}

var
  long  duty[4]
  long  period
  long  halfPeriod
  long  pwmstack1[12]                                           ' Stack size measured with Stack Length = 9 longs
  long  pwmstack2[12]

pub start_pwm(p1, p2, p3, p4, freq) | i
  period := clkfreq / (1 #> freq <# 20_000)                     ' limit pwm frequency
  halfPeriod := period / 2
  cognew(run_pwm(p1, p2, 0), @pwmstack1)                        ' launch 1st pwm cog
  cognew(run_pwm(p3, p4, 2), @pwmstack2)                        ' launch 2nd pwm cog
  
pub set_duty(ch, level)
  level := 0 #> ||level <# 1000                                 ' limit duty cycle
  ch := 0 #> ch <# 3
  duty[ch] := -period * level / 1000

pub get_duty(ch)
  ch := 0 #> ch <# 3
  return duty[ch]
  
pri run_pwm(p1, p2, d) | t                                      ' start with cognew
  if (p1 => 0)
    ctra := (%00100 << 26) | p1                                 ' pwm mode
    frqa := 1                            
    phsa := 0
    dira[p1] := 1                                               ' make pin an output
  if (p2 => 0)
    ctrb := (%00100 << 26) | p2
    frqb := 1
    phsb := 0
    dira[p2] := 1

  if d > 0
    waitcnt(halfPeriod/2 + cnt)  ' start cogs out of phase to spread motor load

  t := cnt

  repeat
    phsa := duty[d]
    waitcnt(t += halfPeriod)     ' split period in half to spread motor load
    phsb := duty[d+1]
    waitcnt(t += halfPeriod)
