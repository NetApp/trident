#!/bin/bash


# ONTAP Role Generator Bash Sample Scripts
# This script was developed by NetApp to help demonstrate
# NetApp technologies. This script is not officially
# supported as a standard NetApp product.

# Purpose: THE FOLLOWING SCRIPT SHOWS HOW TO GENERATE ROLE FOR ONTAP.

# usage: ./role-generator.sh [-h] [-v VSERVER_NAME] [-r ROLE_NAME] [--zapi] [--rest] [-j JSON] [-o OUTPUT]

# Copyright (c) 2024 NetApp, Inc. All Rights Reserved.
# Licensed under the BSD 3-Clause "New or Revised" License (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# https://opensource.org/licenses/BSD-3-Clause

# Default values
DEFAULT_ZAPI_JSON_FILE="./zapi_custom_role.json"
DEFAULT_ZAPI_OUTPUT_FILE="./zapi_custom_role_output.txt"
DEFAULT_REST_JSON_FILE="./rest_custom_role.json"
DEFAULT_REST_OUTPUT_FILE="./rest_custom_role_output.txt"
ZAPI=false
REST=false

# Function to display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -j, --json-file <file>       Path to the JSON file (default: ZAPI=$DEFAULT_ZAPI_JSON_FILE, REST=$DEFAULT_REST_JSON_FILE)"
    echo "  -o, --output-file <file>     Path to the output file (default: ZAPI=$DEFAULT_REST_OUTPUT_FILE, REST=$DEFAULT_REST_OUTPUT_FILE)"
    echo "  -r, --role-name <string>     Name of the role (default: trident)"
    echo "  -v, --vserver-name <string>  Name of the vserver (default: None)"
    echo "  --zapi                       Generate custom role for ZAPI"
    echo "  --rest                       Generate custom role for REST"
    echo "  -h, --help                   Display this help message"
    exit 1
}

# Parse command-line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -j|--json-file) JSON_FILE="$2"; shift 2 ;;
        -o|--output-file) OUTPUT_FILE="$2"; shift 2 ;;
        -r|--role-name) ROLE_NAME="$2"; shift 2 ;;
        -v|--vserver-name) VSERVER_NAME="$2"; shift 2 ;;
        --zapi) ZAPI=true; shift ;;
        --rest) REST=true; shift ;;
        -h|--help) usage ;;
        *) echo "Unknown parameter passed: $1"; usage ;;
    esac
done

if [ "$ZAPI" = true ] && [ "$REST" = true ]; then
    echo "Please specify either -z or -r option"
    usage
fi

if [ "$ZAPI" = false ] && [ "$REST" = false ]; then
    echo "Please specify either -z or -r option"
    usage
fi

# Set default values if not provided
if [ "$ZAPI" = true ]; then
    JSON_FILE="${JSON_FILE:-$DEFAULT_ZAPI_JSON_FILE}"
    OUTPUT_FILE="${OUTPUT_FILE:-$DEFAULT_ZAPI_OUTPUT_FILE}"
else
    JSON_FILE="${JSON_FILE:-$DEFAULT_REST_JSON_FILE}"
    OUTPUT_FILE="${OUTPUT_FILE:-$DEFAULT_REST_OUTPUT_FILE}"
fi

ROLE_NAME="${ROLE_NAME:-trident}"
VSERVER_NAME="${VSERVER_NAME:-None}"

# Clear the output file if it exists
> "$OUTPUT_FILE"


# Read and process the JSON file using jq
if [ "$ZAPI" = true ]; then
  jq -c '.[]' "$JSON_FILE" | while read -r item; do
      command=$(echo "$item" | jq -r '.command')
      access_level=$(echo "$item" | jq -r '.access_level')

      if [ "$VSERVER_NAME" = "None" ]; then
        formatted_line="security login role create -role \"$ROLE_NAME\" -cmddirname \"$command\" -access $access_level"
      else
        formatted_line="security login role create -role \"$ROLE_NAME\" -vserver \"$VSERVER_NAME\" -cmddirname \"$command\" -access $access_level"
      fi

      echo "$formatted_line" >> "$OUTPUT_FILE"
  done
echo "Commands have been written to $OUTPUT_FILE"
else
  jq -c '.[]' "$JSON_FILE" | while read -r item; do
      path=$(echo "$item" | jq -r '.path')
      access=$(echo "$item" | jq -r '.access')

      if [ "$VSERVER_NAME" = "None" ]; then
        formatted_line="security login rest-role create -role \"$ROLE_NAME\" -api \"$path\" -access $access"
      else
        formatted_line="security login rest-role create -role \"$ROLE_NAME\" -vserver \"$VSERVER_NAME\" -api \"$path\" -access $access"
      fi

      echo "$formatted_line" >> "$OUTPUT_FILE"
  done
  echo "Paths have been written to $OUTPUT_FILE"
fi
