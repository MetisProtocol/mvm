import os
import logging
import json
import requests
from flask import Flask
from flask import request

import subprocess
from env import MyEnv


app = Flask(__name__)

SEARCH_DOMAIN = os.environ.get('SEARCH_DOMAIN')

DIR = '/l2-node/l2'

print(os.environ)


@app.route('/')
def health():
    return 'Ok'
    

@app.route('/v1/chain/update',methods=['POST'])
def update_chain():
    mount_path = request.args.get("mount_path")
    efs_id = request.args.get("efs_id")
    access_point_id = request.args.get("access_point_id")
    reset = request.args.get("reset")
    body = request.get_data(as_text=True)
    if reset is not None and len(reset)>0:
        output = _kill_pid(reset, json.loads(body))
    else:
        _kill_pids()
        the_path = mount_path or '/metis'
        logging.warning(f'update_chain mount_path:{mount_path}, efs_id:{efs_id}, access_point_id:{access_point_id}')
        _umount_path(the_path)
        if efs_id is not None and access_point_id is not None:
            output=_try_cmd(['mount', '-t', 'efs', '-o', f'tls,accesspoint={access_point_id}',f'{efs_id}:', the_path])
            logging.warning(f'mount to ap: {efs_id},{access_point_id},{output}')
            _try_cmd(['mkdir', '-p', f'{the_path}/.ethereum'])
            _try_cmd(['mount','--bind',f'{the_path}/.ethereum','/root/.ethereum'])
            logging.warning(f"mount to root ethereum: mount --bind {the_path}/.ethereum /root/.ethereum")
        if output is None or len(output) <= 0:
            output=_update_chain(json.loads(body))
    return {
        'data': output.decode('utf-8')
    }


def _kill_pids():
    output = _try_cmd_string("/app/process_kill.sh")
    logging.warning(output)


def _kill_pids_safe():
    output = _try_cmd(["/app/process_kill.sh", "SIGTERM"])
    logging.warning(output)


def _kill_pid(name, body):
    if name not in ["geth","batch-submitter", "message-relayer"]:
        return f"{name} must be in the ['geth','batch-submitter','message-relayer'] list".encode("utf-8")
    #logging.warning(f'_kill_pid to file:{body}')
    _save_env(body)
    output = _try_cmd(["/app/process_kill.sh", name, "SIGTERM"])
    logging.warning(output)
    return output


def _umount_path(path):
    _try_cmd(['umount', "-fl", '/root/.ethereum'])
    _try_cmd(['umount', "-fl", path])
    output = _try_cmd_string(f"df -h|grep -w {path}")
    logging.warning(f"check umount result: {output}")
    

def _try_cmd(cmds):
    try:
        return subprocess.check_output(cmds)
    except Exception as e:
        logging.warning(f'exce cmd in update chain error => {cmds}')
        return ""
        

def _try_cmd_string(cmd):
    try:
        return subprocess.check_output(cmd, shell=True)
    except Exception as e:
        logging.warning(f'exce cmd string in update chain error => {cmd}')
        return "" 
        

def _update_chain(body):
    #logging.warning(f'update_chain to file:{body}')
    _save_env(body)
    
    output = _try_cmd(['cat','/app/env.sh'])
    #logging.warning(output)
    
    output = _try_cmd([f'/app/restart.sh','/app/env.sh'])
    logging.warning(output)
    return output


def _save_env(body):
    if body is None:
        return False
    myEnv = MyEnv('')
    myEnv.SetEnvFile("/app/env.sh")
    myEnv.envs=body
    myEnv.Save()
    #logging.warning(f'MyEnv to file:{myEnv.envs}')
    return True
    

@app.route('/v1/chain/safe/stop',methods=['POST'])
def safe_stop_chain():
    _kill_pids_safe()
    mount_path = request.args.get("mount_path")
    the_path = mount_path or '/metis'
    logging.warning('safe_stop_chain...')
    _umount_path(the_path)
    return {
        'data': "success"
    } 
    

@app.route('/v1/chain/stop',methods=['POST'])
def stop_chain():
    _kill_pids()
    mount_path = request.args.get("mount_path")
    the_path = mount_path or '/metis'
    logging.warning('stop_chain...')
    _umount_path(the_path)
    return {
        'data': "success"
    }


@app.route('/v1/shell/exec',methods=['POST'])
def exec_shell():
    bodys=request.get_data()
    logging.warning(bodys)
    body = json.loads(bodys)
    cmd = body['cmd']
    # tokens=cmd.split(' ')
    # logging.warning(tokens)
    # ls_output = subprocess.check_output(tokens)
    ls_output = _try_cmd_string(cmd)
    logging.warning(ls_output)
    response = {
        'data': ls_output.decode('utf-8')
    }
    logging.warning(response)
    return response


@app.route('/v1/metis/l2/geth')
def metis_l2_geth():
    logging.warning('mount to ap')
    efs_id = request.args.get("efs_id")
    access_point_id = request.args.get("access_point_id")
    mount_path = request.args.get("mount_path")
    the_path = mount_path or '/metis'
    ls_output=subprocess.check_output(['mount', '-t', 'efs', '-o', f'tls,accesspoint={access_point_id}',f'{efs_id}:', the_path])
    logging.warning(ls_output)
    response = {
        'data': ls_output.decode('utf-8')
    }
    logging.warning(response)
    return response


@app.route('/v1/batch/submitter')
def talk_to_batch_submitter():
    host = f'http://batch_submitter_{SEARCH_DOMAIN}:4567' if SEARCH_DOMAIN else 'http://batch_submitter:4567'
    print(f'Calling batch submitter host: {host}')
    response = requests.get(f'{host}/v1/batch_submitter')
    print(response.content)

    return {
        'data': response.json()
    }, response.status_code


@app.route('/v1/dtl')
def talk_to_dtl():
    host = f'http://data_transport_layer_{SEARCH_DOMAIN}:7878' if SEARCH_DOMAIN else 'http://data_transport_layer:7878'
    print(f'Calling batch submitter host: {host}')
    response = requests.get(f'{host}/v1/dtl')
    print(response.content)

    return {
        'data': response.json()
    }, response.status_code


if __name__ == "__main__" :
    _update_chain({'a':'1','b':'2', 'l2v':'10', 'l2r':'http://ip-172-31-12-82.us-east-2.compute.internal:8089/state-dump.latest.json'})