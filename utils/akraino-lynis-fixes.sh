#!/bin/bash
sudo sysctl -w kernel.dmesg_restrict=1
sudo sysctl -w net.ipv4.conf.default.accept_source_route=0
sudo sed -i '/^PASS_MAX_DAYS/c\PASS_MAX_DAYS   998' /etc/login.defs
sudo echo "AllowUsers core" >> /etc/ssh/sshd_config
sudo echo "AllowGroups core" >> /etc/ssh/sshd_config
sudo sed -i 's/^    umask.*/    umask 027/g' /etc/profile
