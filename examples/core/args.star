#!/usr/bin/env kite
# args.star — Demonstrates CLI flag and positional argument parsing with args.*
#
# Usage examples:
#   kite run ./args.star --help
#   kite run ./args.star payment-service
#   kite run ./args.star payment-service -e prod -r 3 -p -t web,api
#   kite run ./args.star payment-service --environment staging --ratio 0.75
#   kite run ./args.star payment-service --no-preview

# 1. Declare schema (flags and positionals)
args.string(
    "environment",
    shorthand = "e",
    default = "dev",
    choices = ["dev", "staging", "prod"],
    help = "Target deployment environment",
    var_fallback = "env",
)

args.int(
    "replicas",
    shorthand = "r",
    default = 1,
    min = 1,
    max = 100,
    help = "Number of service replicas (1-100)",
)

args.bool(
    "preview",
    shorthand = "p",
    default = False,
    help = "Preview actions without applying changes",
)

args.float(
    "ratio",
    default = 1.0,
    help = "Traffic weight ratio (0.0 to 1.0)",
)

args.list(
    "tags",
    shorthand = "t",
    default = ["web", "backend"],
    help = "Resource tags (repeatable or comma-separated)",
)

args.positional(
    "service-name",
    required = True,
    help = "Name of the target service to deploy",
)

def main():
    # 2. Parse arguments against the declared schema
    opts = args.parse()

    print("Deploying service:", opts.service_name)
    print("  Environment :", opts.environment)
    print("  Replicas    :", opts.replicas)
    print("  Preview     :", opts.preview)
    print("  Ratio       :", opts.ratio)
    print("  Tags        :", opts.tags)

    if opts.preview:
        print("\n[Preview Mode] Validation succeeded; no actual deployment triggered.")
    else:
        print("\nDeployment completed successfully.")
