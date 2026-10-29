from pathlib import Path
import subprocess
task_temp=Path(__file__).parent/'build-temp-03'
task_temp.mkdir(exist_ok=True)
subprocess.run(['compact.exe','/C','/I','/Q',str(task_temp)],check=True,stdout=subprocess.DEVNULL)
source=Path(r'C:\tmp\yak-shark-TLS-metadata-20261028\run_build.py').read_text('utf8')
old="root=Path(r'C:\\tmp\\yak-shark-TLS-metadata-20261028')"
assert source.count(old)==1
source=source.replace(old,"root=Path(r'C:\\tmp\\yak-shark-ESP-SSH-UI-20261028')")
old='out=root/sys.argv[1];out.mkdir(exist_ok=False)'
assert source.count(old)==1
source=source.replace(old,old+"\nsubprocess.run(['compact.exe','/C','/I','/Q',str(out)],check=True,stdout=subprocess.DEVNULL)")
assert source.count('GOTMPDIR=str(temp)')==1
source=source.replace('GOTMPDIR=str(temp)','GOTMPDIR=str(temp),TMP=str(task_temp),TEMP=str(task_temp)')
exec(compile(source,str(Path(__file__)),'exec'),globals())
