import boto3
import logging
import os
from datetime import datetime, timedelta

try:
    from dotenv import find_dotenv, load_dotenv

    load_dotenv(find_dotenv())
except:
    pass

# All codes except functions will be executed when this module is imported

# Get credentials
try:
    AWS_ACCESS_KEY_ID = os.environ["AWS_ACCESS_KEY_ID"]
    AWS_SECRET_ACCESS_KEY = os.environ["AWS_SECRET_ACCESS_KEY"]
    S3_BUCKET = os.environ["S3_BUCKET"]
except KeyError as e:
    logging.error(e.__repr__())
    SystemExit(1)

# access s3 with credentials
session = boto3.session.Session(AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
s3_client = session.client("s3")


def upload_file_to_s3(fpath: str, s3_key: str):
    s3_client.upload_file(fpath, S3_BUCKET, s3_key)
    logging.info(f"Success. File written to {S3_BUCKET}/{s3_key}")


def write_text_to_s3_file(text: str, s3_key: str, exp_days: int = 0):
    if exp_days:
        expiration_time = datetime.now() + timedelta(days=exp_days)
        s3_client.put_object(Bucket=S3_BUCKET, Key=s3_key, Body=text, Expires=expiration_time)
    else:
        s3_client.put_object(Bucket=S3_BUCKET, Key=s3_key, Body=text)

    logging.info(f"Success. Data written to {S3_BUCKET}/{s3_key}")


def create_presigned_url(s3_key: str, exp_days: int = 7) -> str:
    if not exp_days:  # handle 0 expiry days
        exp_days = 7

    expiration_time = datetime.now() + timedelta(days=exp_days)

    presigned_url = s3_client.generate_presigned_url(
        "get_object",
        Params={"Bucket": S3_BUCKET, "Key": s3_key},
        ExpiresIn=int((expiration_time - datetime.now()).total_seconds()),
    )
    logging.info(f"Success. Presigned URL created.")
    return presigned_url
