@echo off
rem Switch the REDMI monitor back to the Mac (HDMI1 = source 23).
rem If the monitor gets a new IP, update it here (or reserve its IP in the router).
curl -s -m 5 "http://192.168.31.90:6095/controller?action=changesource2&sourceid=23" >nul
