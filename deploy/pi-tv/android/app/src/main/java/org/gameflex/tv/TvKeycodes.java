package org.gameflex.tv;
/** Linux scan codes missing from virtual Android DPAD/ADB keyboard events. */
public final class TvKeycodes {
 public static int scan(int key){switch(key){case 19:return 103;case 20:return 108;case 21:return 105;case 22:return 106;case 23:case 66:return 28;case 54:return 44;case 52:return 45;case 29:return 30;case 47:return 31;case 45:return 16;case 51:return 17;case 59:return 42;case 60:return 54;default:return 0;}}
}
