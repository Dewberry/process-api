#!/usr/bin/python
"""
Clips a Raster

Usage: python clip_raster.py '{"jobID": "sadf234sdf234sdf", "resultsDir": "results", "clippedRasterDestination": "clipped-raster-inputs/clipped_raster.tif", "maskLayer": "/vsizip//vsis3/texas-glo/clipped-raster-inputs/maskLayer.zip/maskLayer.shp", "inputRaster": "/vsis3/texas-glo/clipped-raster-inputs/input_raster.tif"}'

Expects last argument to be payload
"""

import logging
import subprocess
import sys
from json import dumps, loads

from utils import S3_BUCKET, create_presigned_url, write_text_to_s3_file

if __name__ == "__main__":

    if len(sys.argv) == 2 and (sys.argv[1] == "--help" or sys.argv[1] == "-h"):
        print(__doc__)
        sys.exit()

    if len(sys.argv) < 2:
        raise KeyError("Not enough arguments")

    # exptected: {"jobID": "sadf234sdf234sdf", "resultsDir": "results", "clippedRasterDestination": "clipped-raster-inputs/clipped_raster.tif", "maskLayer": "/vsizip//vsis3/texas-glo/clipped-raster-inputs/maskLayer.zip/maskLayer.shp", "inputRaster": "/vsis3/texas-glo/clipped-raster-inputs/input_raster.tif"}
    params_string = sys.argv[-1]
    params_dict = loads(params_string)

    subprocess.check_call(
        [
            "gdalwarp",
            "-overwrite",
            "-of",
            "GTiff",
            "-cutline",
            params_dict["maskLayer"],
            "-cl",
            "maskLayer",
            "-crop_to_cutline",
            params_dict["inputRaster"],
            f"/vsis3/{S3_BUCKET}/{params_dict['clippedRasterDestination']}",
        ]
    )

    presigned_url = create_presigned_url(params_dict["clippedRasterDestination"], int(params_dict["expDays"]))

    logging.info("Writing results to S3")
    result = {
        "clippedRaster": {
            "value": params_dict["clippedRasterDestination"],
            "links": [{"href": presigned_url, "type": "application/tif; application/geotiff", "title": "presignedURL"}],
        }
    }
    write_text_to_s3_file(
        dumps(result), f'{params_dict["resultsDir"]}/{params_dict["jobID"]}.json', int(params_dict["expDays"])
    )
