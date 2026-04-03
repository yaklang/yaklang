from base import BaseImage


class Image(BaseImage):
    class Meta(BaseImage.Meta):
        swappable = "FILER_IMAGE_MODEL"
        default_manager_name = "objects"
