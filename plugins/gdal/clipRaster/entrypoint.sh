#!/bin/sh
# Parse inputs and call clip raster

PARAMS=$1

INPUT_RASTER=$( jq -n -r --argjson data "$PARAMS" '$data.inputRaster')
MASK_LAYER=$( jq -n -r --argjson data "$PARAMS" '$data.maskLayer')
CLIPPED_RASTER_DESTINATION=$( jq -n -r --argjson data "$PARAMS" '$data.clippedRasterDestination')
JOB_ID=$( jq -n -r --argjson data "$PARAMS" '$data.jobID')


gdalwarp -overwrite -of GTiff -cutline "${MASK_LAYER}" -cl maskLayer -crop_to_cutline "${INPUT_RASTER}" "${CLIPPED_RASTER_DESTINATION}"

jq -n -j --arg CLIPPED_RASTER_DESTINATION "$CLIPPED_RASTER_DESTINATION" '{clippedRaster: $CLIPPED_RASTER_DESTINATION}' > ${JOB_ID}.json
aws s3 cp ${JOB_ID}.json s3://${S3_BUCKET}/results/ --content-type 'application/json'