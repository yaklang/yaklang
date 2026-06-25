from flask import Flask
from fastapi import APIRouter

app = Flask(__name__)
router = APIRouter()


@app.route('/hello')
def hello():
    return 'ok'


@app.route('/legacy', methods=['POST'])
def legacy():
    pass


@router.get('/items')
def items():
    return []
