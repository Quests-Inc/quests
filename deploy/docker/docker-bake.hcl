# Bake definition for Quests images (Go API + Python bot).
#
# Local:
#   docker buildx bake -f deploy/docker/docker-bake.hcl api
#   docker buildx bake -f deploy/docker/docker-bake.hcl api bot
#
# CI sets API_IMAGE / BOT_IMAGE / TAG / VERSION through the environment (release.yml).

variable "API_IMAGE" {
  default = "quests-api"
}

variable "BOT_IMAGE" {
  default = "quests-bot"
}

# The image tag: the release version in CI (1.4.0), "local" on a workstation.
variable "TAG" {
  default = "local"
}

# The release, stamped into `quests-server version` and /api/health ("dev" outside a release).
variable "VERSION" {
  default = "dev"
}

target "_common" {
  context    = "."
  dockerfile = "deploy/docker/Dockerfile"
  # Same scope so api+bot rebuilds reuse GHA cache for python-base.
  cache-from = ["type=gha,scope=quests"]
  cache-to   = ["type=gha,mode=max,scope=quests"]
}

target "api" {
  inherits = ["_common"]
  target   = "api"
  args     = { VERSION = VERSION }
  tags     = ["${API_IMAGE}:${TAG}"]
}

target "bot" {
  inherits = ["_common"]
  target   = "bot"
  tags     = ["${BOT_IMAGE}:${TAG}"]
}

group "default" {
  targets = ["api", "bot"]
}
