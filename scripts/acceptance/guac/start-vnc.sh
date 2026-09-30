#!/bin/sh
# Xvnc（真 VNC server，VncAuth 口令）+ xterm 提供画面内容
set -e
Xvnc :0 -rfbport 5900 -geometry 1280x800 -depth 24 \
    -SecurityTypes VncAuth -PasswordFile /root/.vnc/passwd -localhost no &
sleep 1
DISPLAY=:0 xterm -geometry 90x28+60+60 -title guac-accept-vnc &
wait
