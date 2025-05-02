package User_Groups

import (
	"Proyecto/Environment"
	"Proyecto/Partitions"
	"Proyecto/Utils"
	"encoding/binary"
	"fmt"
	"strings"
)

func RMGRP(name string) string {
	var output strings.Builder
	output.WriteString("╔══════════════════════ INICIO RMGRP  ═════════════════════════╗ \n")
	output.WriteString(fmt.Sprintf("  Nombre del grupo a eliminar: %s\n", name))

	// Verificar si el usuario actual es root
	if !IsRootLoggedIn() {
		return "❌ Error: Solo el usuario root puede eliminar grupos."
	}

	// Obtener la partición montada actualmente
	mountedPartition := Environment.GetCurrentMountedPartition()
	if mountedPartition == nil {
		return "❌ Error: No hay ninguna partición montada."
	}

	// Abrir el archivo del sistema de archivos
	file, err := Utils.AbrirArchivo(mountedPartition.MountPath)
	if err != nil {
		return fmt.Sprintf("❌ Error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Leer el Superblock de la partición
	var tempSuperblock Partitions.Superblock
	if err := Utils.LeerArchivo(file, &tempSuperblock, int64(mountedPartition.MountStart)); err != nil {
		return fmt.Sprintf("❌ Error al leer el Superblock: %v", err)
	}

	// Buscar el archivo "users.txt"
	indexInode, log := InitSearch("/users.txt", file, tempSuperblock)
	output.WriteString(log) // Agregar el log de InitSearch al log principal
	if indexInode == -1 {
		return "❌ Error: No se encontró el archivo users.txt."
	}

	// Leer el inodo del archivo "users.txt"
	var crrInode Partitions.Inode
	if err := Utils.LeerArchivo(file, &crrInode, int64(tempSuperblock.S_inode_start+indexInode*int32(binary.Size(Partitions.Inode{})))); err != nil {
		return fmt.Sprintf("❌ Error al leer el inodo de users.txt: %v", err)
	}

	blockSize := binary.Size(Partitions.Fileblock{})
	for _, block := range crrInode.I_block {
		if block == -1 {
			continue
		}

		var fileBlock Partitions.Fileblock
		blockOffset := int64(tempSuperblock.S_block_start + block*int32(blockSize))

		if err := Utils.LeerArchivo(file, &fileBlock, blockOffset); err != nil {
			return fmt.Sprintf("❌ Error al leer bloque de archivo: %v", err)
		}

		lines := strings.Split(string(fileBlock.B_content[:]), "\n")
		for i, line := range lines {
			parts := strings.Split(line, ",")
			if len(parts) >= 3 && strings.TrimSpace(parts[1]) == "G" && strings.TrimSpace(parts[2]) == name {
				output.WriteString("✅ Grupo encontrado, marcando como eliminado en su bloque. \n")
				lines[i] = "0,G," + parts[2]
				fileBlock.B_content = [64]byte{}
				copy(fileBlock.B_content[:], strings.Join(lines, "\n"))

				// Escribir el bloque actualizado en el archivo
				if err := Utils.EscribirArchivo(file, fileBlock, blockOffset); err != nil {
					return fmt.Sprintf("❌ Error al escribir el bloque actualizado: %v", err)
				}

				output.WriteString("✅ Grupo eliminado correctamente en su bloque. \n")
				output.WriteString("╚══════════════════════   FIN RMGRP   ═════════════════════════╝ \n")
				return output.String()
			}
		}
	}

	output.WriteString("❌ Error: El grupo no existe en el sistema.\n")
	output.WriteString("╚══════════════════════   FIN RMGRP   ═════════════════════════╝ \n")
	return output.String()
}
