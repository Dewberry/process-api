#!/bin/bash
"""
This script should be called from plugin-examples folder.
"""

# Function to build Docker image
build_image() {
    base_folder="$1"
    sub_folder="$2"
    dockerfile_path="$base_folder/$sub_folder/Dockerfile"

    if [ -f "$dockerfile_path" ]; then
        echo "Building $base_folder/$sub_folder..."
        docker build -t "$sub_folder" "$base_folder/$sub_folder" &
    fi
}

# Iterate over folders and initiate parallel builds
for base_folder in */; do
    base_folder="${base_folder%/}"  # Remove trailing slash
    if [ "$base_folder" != "readme.md" ]; then
        for sub_folder in "$base_folder"/*; do
            sub_folder="$(basename $sub_folder)"
            build_image "$base_folder" "$sub_folder"
        done
    fi
done

# Wait for all parallel jobs to finish
wait
docker images

echo "All builds complete."