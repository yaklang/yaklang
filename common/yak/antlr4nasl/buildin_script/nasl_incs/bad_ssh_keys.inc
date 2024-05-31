###############################################################################
# Collection of known static SSH keys
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2015 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
###############################################################################

# unused key for pre check
check_key = "-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEArdL1ZTVeCu/XnKIM1EfWwv6+ESp2pvgwOM0Lj8IDGOBtMx2R
hHVEcSzgS1OSCNO9XWFm/TTVkt16GMX5vcac8Oo7Bpj9kWL9m6h/fD6wBcGnE7ZM
TgjSUHef3SaiemnQoEe3f+94BOinO02tQk5TJRwFFqEt0BWUddezhgt7Be5vqBPn
p3SwrbcTvmMkw60mb12DOZhIz3/4PgQfRbQn9sVb3qqwQB88cMzUVJLwwhtJ3T52
79WwJLEthGlThxT4Qq4Rlh+tbDISkF2UFbOl3mHJNe2S7bcCGHfVr7YGobtQAm3z
X6gBHIFXRCUm7wcLIhFYthGXgdRTnLEGyAnZuQIDAQABAoIBAQCNF4tw11y6a+wL
i832EUnVhYkpzdn5tcMOmAEmc0ioTKLWts2DQtyHMPHusqr610QCJCnApb4mM/gD
W89Xp3uEYdv/gvdvp3ZLfv3cD9Y5FfNUIMjXHnn4LuFoVQ/VZUaekxfzlKFK+r2j
eW2FQKZknNXUhIf3O00CSk/CIld9e7FqcZNNEe14RNhlrnpzZDe9UNabkIOzd5r6
eBL5sIYbzDQcwwfNlGkghd+bkILnAit/5k9j05zBl5RQZ+gCNOPXwHKcCKz4+AZE
d/ZtoCss/s4vm5HDIwRC66ACdd+sfh5JPNC7xxA+Rw+ICHSr/kHF22OuV77jJmbQ
Tna1tCfBAoGBAOMdr/o5T9GQlG+FIU/aHN3rZSrWG0op7cI7plJhRD8jHOIz9MLy
4gjSJwsR4s0iHeDpVmNn6hZMWwex+az16NREJIJQ2Z63UiEPvTPwG7ockmo21Rph
O1AlxMyfeArejIgSXs3TJe+s+O+66iGr5Xw8HyxQ0AmoyqM1C3TL2+x9AoGBAMPu
OTRLX5b5oMLvDjQmdNZqexuiksT+kLKe91jizaflfce4HMs/DRV9WcTdW5VLxJK7
zfowL4K4dSZNpsx1lV49ii77YMVghLQ/eaaXti1paBAF67WEs95FPfb1dPgc5Xcc
pT7NKxk0v3ztO2vhJ+valUlvTp5ehwN5FKoL1bLtAoGBANVOXKk2irDYiZD0CIVm
5RXIsAu7a4YMOzHhtCRLFj32THpnMcqnM4P1XJYZBlqDYGyMuoZCYqjurdBcZ5L6
ouI53dBG/x0CMDC9B4ZwTJpPm3QGa7+K8poj3uIQJD6RJovmNQQ6+7ZPTr5ZXuyg
9bfo/54viDSKQ0dl2oynqrZBAoGAM0Wd1p96zklY1mo0b1U48fs/aAXtHnv3ZabP
14TSiIM3nRIDBvTi+aKp4SiEFNeVA0j+J+CALlcS4QklAxAqHFVCWfUSZ9RQJdMO
hJfi5pNU1gyWmBO39gxsoLmu+FanfboTTBHom1oAWjLCgAKVrdSviF4GVs8pybDf
xXf/XikCgYBR5gXG+KvV3n8JGqsvBfd23o25OdDRBBIWOaOZPduWBz6pqIa9v5an
ciC7JBBvlcKkovidTe7tjJksn6d+vDuc21Iygfbd5VWZB8ngFqlxIxaAkOoK4RVR
b/JeWCQd9v/CiuH6imprxSTPdfMPx7aL7w+N3YAeRuxXCBAj6ek3jg==
-----END RSA PRIVATE KEY---";

bad_keys = make_list( "sync:split:-----BEGIN DSA PRIVATE KEY-----
MIIBugIBAAKBgQCUw7F/vKJT2Xsq+fIPVxNC/Dyk+dN9DWQT5RO56eIQasd+h6Fm
q1qtQrJ/DOe3VjfUrSm7NN5NoIGOrGCSuQFthFmq+9Lpt6WIykB4mau5iE5orbKM
xTfyu8LtntoikYKrlMB+UrmKDidvZ+7oWiC14imT+Px/3Q7naj0UmOrSTwIVAO25
Yf3SYNtTYv8yzaV+X9yNr/AfAoGADAcEh2bdsrDhwhXtVi1L3cFQx1KpN0B07JLr
gJzJcDLUrwmlMUmrXR2obDGfVQh46EFMeo/k3IESw2zJUS58FJW+sKZ4noSwRZPq
mpBnERKpLOTcWMxUyV8ETsz+9oz71YEMjmR1qvNYAopXf5Yy+4Zq3bgqmMMQyM+K
O1PdlCkCgYBmhSl9CVPgVMv1xO8DAHVhM1huIIK8mNFrzMJz+JXzBx81ms1kWSeQ
OC/nraaXFTBlqiQsvB8tzr4xZdbaI/QzVLKNAF5C8BJ4ScNlTIx1aZJwyMil8Nzb
+0YAsw5Ja+bEZZvEVlAYnd10qRWrPeEY1txLMmX3wDa+JvJL7fmuBgIUZoXsJnzs
+sqSEhA35Le2kC4Y1/A=
-----END DSA PRIVATE KEY-----", #  array-networks-vapv-vxag
                       "mateidu:split:-----BEGIN RSA PRIVATE KEY-----
MIICWwIBAAKBgQDBEh0OUdoiplc0P+XW8VPu57etz8O9eHbLHkQW27EZBEdXEYxr
MOFXi+PkA0ZcNDBRgjSJmHpo5WsPLwj/L3/L5gMYK+yeqsNu48ONbbqzZsFdaBQ+
IL3dPdMDovYo7GFVyXuaWMQ4hgAJEc+kk1hUaGKcLENQf0vEyt01eA/k6QIBIwKB
gQCwhZbohVm5R6AvxWRsv2KuiraQSO16B70ResHpA2AW31crCLrlqQiKjoc23mw3
CyTcztDy1I0stH8j0zts+DpSbYZnWKSb5hxhl/w96yNYPUJaTatgcPB46xOBDsgv
4Lf4GGt3gsQFvuTUArIf6MCJiUn4AQA9Q96QyCH/g4mdiwJBAPHdYgTDiQcpUAbY
SanIpq7XFeKXBPgRbAN57fTwzWVDyFHwvVUrpqc+SSwfzhsaNpE3IpLD9RqOyEr6
B8YrC2UCQQDMWrUeNQsf6xQer2AKw2Q06bTAicetJWz5O8CF2mcpVFYc1VJMkiuV
93gCvQORq4dpApJYZxhigY4k/f46BlU1AkAbpEW3Zs3U7sdRPUo/SiGtlOyO7LAc
WcMzmOf+vG8+xesCDOJwIj7uisaIsy1/cLXHdAPzhBwDCQDyoDtnGty7AkEAnaUP
YHIP5Ww0F6vcYBMSybuaEN9Q5KfXuPOUhIPpLoLjWBJGzVrRKou0WeJElPIJX6Ll
7GzJqxN8SGwqhIiK3wJAOQ2Hm068EicG5WQoS+8+KIE/SVHWmFDvet+f1vgDchvT
uPa5zx2eZ2rxP1pXHAdBSgh799hCF60eZZtlWnNqLg==
-----END RSA PRIVATE KEY-----", # ceragon-fibeair-cve-2015-0936
                       "root:split:-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEA6NF8iallvQVp22WDkTkyrtvp9eWW6A8YVr+kz4TjGYe7gHzI
w+niNltGEFHzD8+v1I2YJ6oXevct1YeS0o9HZyN1Q9qgCgzUFtdOKLv6IedplqoP
kcmF0aYet2PkEDo3MlTBckFXPITAMzF8dJSIFo9D8HfdOV0IAdx4O7PtixWKn5y2
hMNG0zQPyUecp4pzC6kivAIhyfHilFR61RGL+GPXQ2MWZWFYbAGjyiYJnAmCP3NO
Td0jMZEnDkbUvxhMmBYSdETk1rRgm+R4LOzFUGaHqHDLKLX+FIPKcF96hrucXzcW
yLbIbEgE98OHlnVYCzRdK8jlqm8tehUc9c9WhQIBIwKCAQEA4iqWPJXtzZA68mKd
ELs4jJsdyky+ewdZeNds5tjcnHU5zUYE25K+ffJED9qUWICcLZDc81TGWjHyAqD1
Bw7XpgUwFgeUJwUlzQurAv+/ySnxiwuaGJfhFM1CaQHzfXphgVml+fZUvnJUTvzf
TK2Lg6EdbUE9TarUlBf/xPfuEhMSlIE5keb/Zz3/LUlRg8yDqz5w+QWVJ4utnKnK
iqwZN0mwpwU7YSyJhlT4YV1F3n4YjLswM5wJs2oqm0jssQu/BT0tyEXNDYBLEF4A
sClaWuSJ2kjq7KhrrYXzagqhnSei9ODYFShJu8UWVec3Ihb5ZXlzO6vdNQ1J9Xsf
4m+2ywKBgQD6qFxx/Rv9CNN96l/4rb14HKirC2o/orApiHmHDsURs5rUKDx0f9iP
cXN7S1uePXuJRK/5hsubaOCx3Owd2u9gD6Oq0CsMkE4CUSiJcYrMANtx54cGH7Rk
EjFZxK8xAv1ldELEyxrFqkbE4BKd8QOt414qjvTGyAK+OLD3M2QdCQKBgQDtx8pN
CAxR7yhHbIWT1AH66+XWN8bXq7l3RO/ukeaci98JfkbkxURZhtxV/HHuvUhnPLdX
3TwygPBYZFNo4pzVEhzWoTtnEtrFueKxyc3+LjZpuo+mBlQ6ORtfgkr9gBVphXZG
YEzkCD3lVdl8L4cw9BVpKrJCs1c5taGjDgdInQKBgHm/fVvv96bJxc9x1tffXAcj
3OVdUN0UgXNCSaf/3A/phbeBQe9xS+3mpc4r6qvx+iy69mNBeNZ0xOitIjpjBo2+
dBEjSBwLk5q5tJqHmy/jKMJL4n9ROlx93XS+njxgibTvU6Fp9w+NOFD/HvxB3Tcz
6+jJF85D5BNAG3DBMKBjAoGBAOAxZvgsKN+JuENXsST7F89Tck2iTcQIT8g5rwWC
P9Vt74yboe2kDT531w8+egz7nAmRBKNM751U/95P9t88EDacDI/Z2OwnuFQHCPDF
llYOUI+SpLJ6/vURRbHSnnn8a/XG+nzedGH5JGqEJNQsz+xT2axM0/W/CRknmGaJ
kda/AoGANWrLCz708y7VYgAtW2Uf1DPOIYMdvo6fxIB5i9ZfISgcJ/bbCUkFrhoH
+vq/5CIWxCPp0f85R4qxxQ5ihxJ0YDQT9Jpx4TMss4PSavPaBH3RXow5Ohe+bYoQ
NE5OgEXk2wVfZczCZpigBKbKZHNYcelXtTt/nP3rsCuGcM4h53s=
-----END RSA PRIVATE KEY-----", # vagrant
                       "cluster:split:----BEGIN DSA PRIVATE KEY-----
MIIBuwIBAAKBgQDKuRHCBXXwoyWpMkJz/wQafaHOpqWmVYLn9GZ6eQuNLcIhtZQE
kCWZTNajgf4ZAVmHgQh1JHDixJ1V0mcweti/lvyxiqHap7IkD0a+ewAOoz3OpjQZ
3ox2ovHEnQJfZ/9LNiEI3XK8TPAj6trhMn5tCdwFei6228a+TYBOccTPgwIVAKYW
T8ztHHaN7Gwn0I6keQfBSNw1AoGAHYNfKAcqf7Y4wyoVoZpr/h21SETpEaksQb7h
GRJnFpYN/JiyE9W8nX6UqLv1eKyOXLccAnyda0a+uqcOhsAq8+H15slZYa4+065L
ckPfs0V4cpxeMHTT1hK4TR2/LRpUjhYjgXFE5aLl91f5Gug5HemUK2S0BWh/oI38
k2WfNh0CgYEArsJgp7RLPOsCeLqoia/eljseBFVDazO5Q0ysUotTw9wgXGGVWREw
m8wNggFNb9eCiBAAUfVZVfhVAtFT0pBf/eIVLPXyaMw3prBt7LqeBrbagODc3WAA
dMTPIdYYcOKgv+YvTXa51zG64v6pQOfS8WXgKCzDl44puXfYeDk5lVQCFAPfgalL
+FT93tofXMuNVfeQMLJl
-----END DSA PRIVATE KEY-----"); # barracuda load balancer CVE-2014-8428

