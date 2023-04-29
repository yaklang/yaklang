#!/bin/bash
##Filename:     CentOS_Check_Script.sh
##Date:         2019-03-01
##Description:  Security detection script
##https://github.com/xiaoyunjie/Shell_Script/blob/master/Check_script/bash/CentOS_Check_Script.sh

echo "##########################################################################"
echo "#                                                                        #"
echo "#                        Epoint health check script                      #"
echo "#                                                                        #"
echo "#警告:本脚本只是一个检查的操作,未对服务器做任何修改,管理员可以根据此报告 #"
echo "#进行相应的安全整改                                                      #"
echo "##########################################################################"
echo " "
#read -p "=====================Are You Ready,Please press enter=================="
echo " "
echo "##########################################################################"
echo "#                                                                        #"
echo "#                               主机安全检测                             #"
echo "#                                                                        #"
echo "##########################################################################"
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>系统基本信息<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
hostname=$(uname -n)
system=$(cat /etc/os-release | grep "^NAME" | awk -F\" '{print $2}')
version=$(cat /etc/redhat-release | awk '{print $4$5}')
kernel=$(uname -r)
platform=$(uname -p)
address=$(ip addr | grep inet | grep -v "inet6" | grep -v "127.0.0.1" | awk '{ print $2; }' | tr '\n' '\t' )
cpumodel=$(cat /proc/cpuinfo | grep name | cut -f2 -d: | uniq)
cpu=$(cat /proc/cpuinfo | grep 'processor' | sort | uniq | wc -l)
machinemodel=$(dmidecode | grep "Product Name" | sed 's/^[ \t]*//g' | tr '\n' '\t' )
date=$(date)

echo "主机名:           $hostname"
echo "系统名称:         $system"
echo "系统版本:         $version"
echo "内核版本:         $kernel"
echo "系统类型:         $platform"
echo "本机IP地址:       $address"
echo "CPU型号:          $cpumodel"
echo "CPU核数:          $cpu"
echo "机器型号:         $machinemodel"
echo "系统时间:         $date"
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>资源使用情况<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
summemory=$(free -h |grep "Mem:" | awk '{print $2}')
freememory=$(free -h |grep "Mem:" | awk '{print $4}')
usagememory=$(free -h |grep "Mem:" | awk '{print $3}')
uptime=$(uptime | awk '{print $2" "$3" "$4" "$5}' | sed 's/,$//g')
loadavg=$(uptime | awk '{print $9" "$10" "$11" "$12" "$13}')

echo "总内存大小:           $summemory"
echo "已使用内存大小:       $usagememory"
echo "可使用内存大小:       $freememory"
echo "系统运行时间:         $uptime"
echo "系统负载:             $loadavg"
echo "=============================dividing line================================"
echo "内存状态:"
vmstat 2 5
echo "=============================dividing line================================"
echo "僵尸进程:"
ps -ef | grep zombie | grep -v grep
if [ $? == 1 ];then
    echo ">>>无僵尸进程"
else
    echo ">>>有僵尸进程------[需调整]"
fi
echo "=============================dividing line================================"
echo "耗CPU最多的进程:"
ps auxf |sort -nr -k 3 |head -5
echo "=============================dividing line================================"
echo "耗内存最多的进程:"
ps auxf |sort -nr -k 4 |head -5
echo "=============================dividing line================================"
echo  "环境变量:"
env
echo "=============================dividing line================================"
echo  "路由表:"
route -n
echo "=============================dividing line================================"
echo  "监听端口:"
netstat -tunlp
echo "=============================dividing line================================"
echo  "当前建立的连接:"
netstat -n | awk '/^tcp/ {++S[$NF]} END {for(a in S) print a, S[a]}'
echo "=============================dividing line================================"
echo "开机启动的服务:"
systemctl list-unit-files | grep enabled
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>系统用户情况<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo  "活动用户:"
w | tail -n +2
echo "=============================dividing line================================"
echo  "系统所有用户:"
cut -d: -f1,2,3,4 /etc/passwd
echo "=============================dividing line================================"
echo  "系统所有组:"
cut -d: -f1,2,3 /etc/group
echo "=============================dividing line================================"
echo  "当前用户的计划任务:"
crontab -l
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>身份鉴别安全<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
grep -i "^password.*requisite.*pam_cracklib.so" /etc/pam.d/system-auth  > /dev/null
if [ $? == 0 ];then
    echo ">>>密码复杂度:已设置"
else
    grep -i "pam_pwquality\.so" /etc/pam.d/system-auth > /dev/null
    if [ $? == 0 ];then
	echo ">>>密码复杂度:已设置"
    else
	echo ">>>密码复杂度:未设置,请加固密码--------[需调整]"
    fi
fi
echo "=============================dividing line================================"
awk -F":" '{if($2!~/^!|^*/){print ">>>("$1")" " 是一个未被锁定的账户,请管理员检查是否是可疑账户--------[需调整]"}}' /etc/shadow
echo "=============================dividing line================================"
more /etc/login.defs | grep -E "PASS_MAX_DAYS" | grep -v "#" |awk -F' '  '{if($2!=90){print ">>>密码过期天数是"$2"天,请管理员改成90天------[需调整]"}}'
echo "=============================dividing line================================"
grep -i "^auth.*required.*pam_tally2.so.*$" /etc/pam.d/sshd  > /dev/null
if [ $? == 0 ];then
  echo ">>>登入失败处理:已开启"
else
  echo ">>>登入失败处理:未开启,请加固登入失败锁定功能----------[需调整]"
fi
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>访问控制安全<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "系统中存在以下非系统默认用户:"
more /etc/passwd |awk -F ":" '{if($3>500){print ">>>/etc/passwd里面的"$1 "的UID为"$3"，该账户非系统默认账户，请管理员确认是否为可疑账户--------[需调整]"}}'
echo "=============================dividing line================================"
echo "系统特权用户:"
awk -F: '$3==0 {print $1}' /etc/passwd
echo "=============================dividing line================================"
echo "系统中空口令账户:"
awk -F: '($2=="!!") {print $1"该账户为空口令账户，请管理员确认是否为新增账户，如果为新建账户，请配置密码-------[需调整]"}' /etc/shadow
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>安全审计<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "正常情况下登录到本机30天内的所有用户的历史记录:"
last | head -n 30
echo "=============================dividing line================================"
echo "查看syslog日志审计服务是否开启:"
if service rsyslog status | egrep " active \(running";then
  echo ">>>经分析,syslog服务已开启"
else
  echo ">>>经分析,syslog服务未开启，建议通过service rsyslog start开启日志审计功能---------[需调整]"
fi
echo "=============================dividing line================================"
echo "查看syslog日志是否开启外发:"
if more /etc/rsyslog.conf | egrep "@...\.|@..\.|@.\.|\*.\* @...\.|\*\.\* @..\.|\*\.\* @.\.";then
  echo ">>>经分析,客户端syslog日志已开启外发--------[需调整]"
else
  echo ">>>经分析,客户端syslog日志未开启外发---------[无需调整]"
fi
echo "=============================dividing line================================"
echo "审计的要素和审计日志:"
more /etc/rsyslog.conf  | grep -v "^[$|#]" | grep -v "^$"
echo "=============================dividing line================================"
echo "系统中关键文件修改时间:"
ls -ltr /bin/ls /bin/login /etc/passwd  /bin/ps /etc/shadow|awk '{print ">>>文件名："$9"  ""最后修改时间："$6" "$7" "$8}'
echo "
###############################################################################################
#   ls文件:是存储ls命令的功能函数,被删除以后,就无法执行ls命令                                 #
#   login文件:login是控制用户登录的文件,一旦被篡改或删除,系统将无法切换用户或登陆用户         #
#   /etc/passwd是一个文件,主要是保存用户信息                                                  #
#   /bin/ps 进程查看命令功能支持文件,文件损坏或被更改后,无法正常使用ps命令                    #
#   /etc/shadow是/etc/passwd的影子文件,密码存放在该文件当中,并且只有root用户可读              #
###############################################################################################"
echo "=============================dividing line================================"
echo "检查重要日志文件是否存在:"
log_secure=/var/log/secure
log_messages=/var/log/messages
log_cron=/var/log/cron
log_boot=/var/log/boot.log
log_dmesg=/var/log/dmesg
if [ -e "$log_secure" ]; then
  echo  ">>>/var/log/secure日志文件存在"
else
  echo  ">>>/var/log/secure日志文件不存在------[需调整]"
fi
if [ -e "$log_messages" ]; then
  echo  ">>>/var/log/messages日志文件存在"
else
  echo  ">>>/var/log/messages日志文件不存在------[需调整]"
fi
if [ -e "$log_cron" ]; then
  echo  ">>>/var/log/cron日志文件存在"
else
  echo  ">>>/var/log/cron日志文件不存在--------[需调整]"
fi
if [ -e "$log_boot" ]; then
  echo  ">>>/var/log/boot.log日志文件存在"
else
  echo  ">>>/var/log/boot.log日志文件不存在--------[需调整]"
fi
if [ -e "$log_dmesg" ]; then
  echo  ">>>/var/log/dmesg日志文件存在"
else
  echo  ">>>/var/log/dmesg日志文件不存在--------[需调整]"
fi
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>剩余信息保护<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "分区情况:"
echo "如果磁盘空间利用率过高，请及时调整---------[需调整]"
df -h
echo "=============================dividing line================================"
echo "可用块设备信息:"
lsblk
echo "=============================dividing line================================"
echo "文件系统信息:"
more /etc/fstab  | grep -v "^#" | grep -v "^$"
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>入侵防范安全<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "系统入侵行为:"
more /var/log/secure |grep refused
if [ $? == 0 ];then
    echo "有入侵行为，请分析处理--------[需调整]"
else
    echo ">>>无入侵行为"
fi
echo "=============================dividing line================================"
echo "用户错误登入列表:"
lastb | head > /dev/null
if [ $? == 1 ];then
    echo ">>>无用户错误登入列表"
else
    echo ">>>用户错误登入--------[需调整]"
    lastb | head
fi
echo "=============================dividing line================================"
echo "ssh暴力登入信息:"
more /var/log/secure | grep  "Failed" > /dev/null
if [ $? == 1 ];then
    echo ">>>无ssh暴力登入信息"
else
    more /var/log/secure|awk '/Failed/{print $(NF-3)}'|sort|uniq -c|awk '{print ">>>登入失败的IP和尝试次数: "$2"="$1"次---------[需调整]";}'
fi
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>恶意代码防范<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "检查是否安装病毒软件:"
crontab -l | grep clamscan.sh > /dev/null
if [ $? == 0 ];then
  echo ">>>已安装ClamAV杀毒软件"
  crontab -l | grep freshclam.sh > /dev/null
  if [ $? == 0 ];then
    echo ">>>已部署定时更新病毒库"
  fi
else
  echo ">>>未安装ClamAV杀毒软件,请部署杀毒软件加固主机防护--------[无需调整]"
fi
echo " "
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>资源控制安全<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
echo "查看是否开启了xinetd服务:"
if ps -elf |grep xinet |grep -v "grep xinet";then
  echo ">>>xinetd服务正在运行，请检查是否可以把xinetd服务关闭--------[无需调整]"
else
  echo ">>>xinetd服务未开启-------[无需调整]"
fi
echo "=============================dividing line================================"
echo  "查看是否开启了ssh服务:"
if service sshd status | grep -E "listening on|active \(running\)"; then
  echo ">>>SSH服务已开启"
else
  echo ">>>SSH服务未开启--------[需调整]"
fi
echo "=============================dividing line================================"
echo "查看是否开启了Telnet-Server服务:"
if more /etc/xinetd.d/telnetd 2>&1|grep -E "disable=no"; then
  echo ">>>Telnet-Server服务已开启"
else
  echo ">>>Telnet-Server服务未开启--------[无需调整]"
fi
echo "=============================dividing line================================"
ps axu | grep iptables | grep -v grep || ps axu | grep firewalld | grep -v grep
if [ $? == 0 ];then
  echo ">>>防火墙已启用"
iptables -nvL --line-numbers
else
  echo ">>>防火墙未启用--------[需调整]"
fi
echo "=============================dividing line================================"
echo  "查看系统SSH远程访问设置策略(host.deny拒绝列表):"
if more /etc/hosts.deny | grep -E "sshd"; then
  echo ">>>远程访问策略已设置--------[需调整]"
else
  echo ">>>远程访问策略未设置--------[无需调整]"
fi
echo "=============================dividing line================================"
echo "查看系统SSH远程访问设置策略(hosts.allow允许列表):"
if more /etc/hosts.allow | grep -E "sshd"; then
  echo ">>>远程访问策略已设置--------[需调整]"
else
  echo ">>>远程访问策略未设置--------[无需调整]"
fi
echo "=============================dividing line================================"
echo "当hosts.allow和host.deny相冲突时,以hosts.allow设置为准"
echo "=============================dividing line================================"
grep -i "TMOUT" /etc/profile /etc/bashrc
if [ $? == 0 ];then
    echo ">>>已设置登入超时限制"
else
    echo ">>>未设置登入超时限制,请设置,设置方法:在/etc/profile或者/etc/bashrc里面添加参数TMOUT=600 --------[需调整]"
fi
echo ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>end<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"