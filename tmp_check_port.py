import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('152.53.179.54', port=22122, username='root', password='QjIdwdcp5R1Nta3', timeout=10)

# 38099 已经绑定 0.0.0.0，外部应该能访问。检查防火墙
si, so, se = c.exec_command('ss -tlnp | grep 38099')
print('Port:', so.read().decode().strip())

# 检查 iptables
si, so, se = c.exec_command('iptables -L INPUT -n 2>/dev/null | grep 38099; ufw status 2>/dev/null | head -5')
print('Firewall:', so.read().decode().strip())

c.close()
