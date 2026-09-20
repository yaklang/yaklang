#include <arpa/inet.h>
#include <rfb/rfb.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char **argv) {
  int w = 64, h = 48, bpp = 4;
  char *pass = getenv("VNC_PASSWORD");
  char *sec = getenv("VNC_SECURITY");
  rfbScreenInfoPtr screen = rfbGetScreen(&argc, argv, w, h, 8, 3, bpp);
  if (!screen) {
    return 1;
  }
  screen->frameBuffer = calloc((size_t)w * (size_t)h * (size_t)bpp, 1);
  if (!screen->frameBuffer) {
    return 1;
  }
  screen->port = 5900;
  screen->ipv6port = 5900;
  screen->listenInterface = htonl(INADDR_ANY);
  screen->alwaysShared = TRUE;
  screen->neverShared = FALSE;
  screen->desktopName = "yak-libvnc";
  if (sec && strcmp(sec, "None") == 0) {
    screen->authPasswdData = NULL;
    screen->passwordCheck = NULL;
  } else {
    static char *list[2];
    if (!pass || !*pass) {
      pass = "LibVncPass!";
    }
    list[0] = pass;
    list[1] = NULL;
    screen->authPasswdData = list;
    screen->passwordCheck = rfbCheckPasswordByList;
  }
  rfbInitServer(screen);
  rfbRunEventLoop(screen, -1, FALSE);
  return 0;
}
