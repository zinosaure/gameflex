/* Le témoin VULKAN — il prouve UNE chose : que le chargeur de l'invité trouve
 * un pilote et énumère un périphérique.
 *
 * POURQUOI IL EXISTE. Le 2026-09-23, on a découvert que le manifeste ICD livré
 * dans le bundle portait un chemin ABSOLU de la machine de construction
 * (`/usr/lib/<triplet>/libvulkan_lvp.so`), dossier qui n'existe sur aucun
 * appareil. Le chargeur répondait « Found no drivers! » : l'invité glibc n'avait
 * AUCUN périphérique Vulkan, pas même lavapipe, et personne ne s'en était
 * aperçu parce que le registre EGL voisin, lui, était sain — donc OpenGL
 * marchait.
 *
 * Ce qui a rendu le défaut si long à voir : aucun de nos témoins ne touchait à
 * Vulkan (`witness` prouve la libc, `witness-mt` les fils, `draw-xcb` X11), et
 * les invités réels sont de MAUVAIS instruments — RPCS3 affiche sa fenêtre
 * d'accueil AVANT d'initialiser le moindre moteur de rendu, donc il ne dit
 * jamais rien de Vulkan tant qu'aucun jeu n'est chargé. On mesurait un silence
 * et on en tirait des conclusions.
 *
 * Désormais la question « l'invité a-t-il Vulkan ? » est une commande.
 *
 * Sortie, sur une seule ligne, comme les autres témoins :
 *   vulkan-witness-ok — 1 périphérique : llvmpipe (LLVM 15.0.6, 256 bits) [CPU, API 1.3.230]
 *   vulkan-witness-ko — vkCreateInstance = -9 (aucun pilote trouvé)
 *
 * Code de sortie : 0 si au moins un périphérique, 1 sinon. Un script peut donc
 * s'y fier sans lire le texte.
 */
#include <stdio.h>
#include <stdlib.h>
#include <vulkan/vulkan.h>

static const char *kind(VkPhysicalDeviceType type) {
    switch (type) {
        case VK_PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU: return "GPU intégré";
        case VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU:   return "GPU discret";
        case VK_PHYSICAL_DEVICE_TYPE_VIRTUAL_GPU:    return "GPU virtuel";
        case VK_PHYSICAL_DEVICE_TYPE_CPU:            return "CPU";
        default:                                     return "autre";
    }
}

int main(void) {
    /* Aucune extension, aucune couche : on ne teste QUE la découverte du
     * pilote. Demander quoi que ce soit de plus mélangerait deux échecs
     * possibles — « pas de pilote » et « pilote sans cette extension ». */
    VkApplicationInfo app = {0};
    app.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO;
    app.pApplicationName = "gameflex-vulkan-witness";
    app.apiVersion = VK_API_VERSION_1_0;

    VkInstanceCreateInfo info = {0};
    info.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO;
    info.pApplicationInfo = &app;

    VkInstance instance = VK_NULL_HANDLE;
    VkResult rc = vkCreateInstance(&info, NULL, &instance);
    if (rc != VK_SUCCESS) {
        /* -9 = VK_ERROR_INCOMPATIBLE_DRIVER : c'est la signature exacte d'un
         * manifeste qui pointe un fichier absent. */
        printf("vulkan-witness-ko — vkCreateInstance = %d%s\n", rc,
               rc == VK_ERROR_INCOMPATIBLE_DRIVER ? " (aucun pilote trouvé)" : "");

        return 1;
    }

    uint32_t count = 0;
    rc = vkEnumeratePhysicalDevices(instance, &count, NULL);
    if (rc != VK_SUCCESS || count == 0) {
        printf("vulkan-witness-ko — instance créée, mais %u périphérique (rc=%d)\n", count, rc);
        vkDestroyInstance(instance, NULL);

        return 1;
    }

    VkPhysicalDevice *devices = calloc(count, sizeof(*devices));
    if (devices == NULL) {
        printf("vulkan-witness-ko — mémoire insuffisante pour %u périphériques\n", count);
        vkDestroyInstance(instance, NULL);

        return 1;
    }
    vkEnumeratePhysicalDevices(instance, &count, devices);

    printf("vulkan-witness-ok — %u périphérique%s :", count, count > 1 ? "s" : "");
    for (uint32_t i = 0; i < count; i++) {
        VkPhysicalDeviceProperties props;
        vkGetPhysicalDeviceProperties(devices[i], &props);
        printf("%s %s [%s, API %u.%u.%u]", i ? "," : "", props.deviceName, kind(props.deviceType),
               VK_VERSION_MAJOR(props.apiVersion), VK_VERSION_MINOR(props.apiVersion),
               VK_VERSION_PATCH(props.apiVersion));
    }
    printf("\n");

    free(devices);
    vkDestroyInstance(instance, NULL);

    return 0;
}
