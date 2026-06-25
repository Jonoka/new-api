import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('152.53.179.54', port=22122, username='root', password='QjIdwdcp5R1Nta3', timeout=10)

si, so, se = c.exec_command('ufw allow 38099/tcp')
print(so.read().decode().strip())
print(se.read().decode().strip())

c.close()
