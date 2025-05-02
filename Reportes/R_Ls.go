package Reportes

import (
	"Proyecto/Environment"
	"Proyecto/Partitions"
	"Proyecto/User_Groups"
	"Proyecto/Utils"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func GenerarReporteLS(pathFileLs string, outputPath string, id string) string {
	if pathFileLs == "" {
		pathFileLs = "/"
	}

	// Verificar que hay una sesión activa
	if !User_Groups.IsUserLoggedIn() {
		return "Error: No hay una sesión activa. Use 'login' primero."
	}

	// Obtener la partición montada por ID
	mountedPartition, found := Environment.GetMountedPartitionByID(id)
	if !found {
		return fmt.Sprintf("Error: No se encontró la partición con ID %s montada", id)
	}

	// Generar contenido LS en formato DOT
	dotContent, err := generateLSDotContent(*mountedPartition, pathFileLs)
	if err != nil {
		return fmt.Sprintf("Error al generar contenido LS: %v", err)
	}

	// Crear directorio de salida si no existe
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Sprintf("Error al crear directorios de salida: %v", err)
	}

	// Escribir archivo DOT
	dotPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotPath, []byte(dotContent), 0644); err != nil {
		return fmt.Sprintf("Error al escribir archivo DOT: %v", err)
	}

	// Generar imagen con Graphviz
	cmd := exec.Command("dot", "-Tjpg", dotPath, "-o", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("Error al generar imagen con Graphviz: %v\nSalida: %s", err, string(output))
	}

	return fmt.Sprintf("Reporte LS generado exitosamente en: %s", outputPath)
}
func generateLSDotContent(partition Environment.MountedPartition, dirPath string) (string, error) {
	file, err := Utils.AbrirArchivo(partition.MountPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo abrir el disco: %v", err)
	}
	defer file.Close()

	// Leer superbloque
	var superblock Partitions.Superblock
	if err := Utils.LeerArchivo(file, &superblock, int64(partition.MountStart)); err != nil {
		return "", fmt.Errorf("no se pudo leer el superbloque: %v", err)
	}

	// Buscar directorio (el inodo raíz siempre es 0 para "/")
	var inodeIndex int32
	var searchLog string
	if dirPath == "/" {
		inodeIndex = 0 // El inodo raíz siempre es 0 en EXT2
	} else {
		inodeIndex, searchLog = User_Groups.InitSearch(dirPath, file, superblock)
		if inodeIndex == -1 {
			return "", fmt.Errorf("error al buscar directorio: %s", searchLog)
		}
	}

	// Leer inodo del directorio
	var dirInode Partitions.Inode
	inodePos := superblock.S_inode_start + inodeIndex*int32(binary.Size(Partitions.Inode{}))
	if err := Utils.LeerArchivo(file, &dirInode, int64(inodePos)); err != nil {
		return "", fmt.Errorf("error al leer inodo en posición %d: %v", inodePos, err)
	}

	// Verificación especial para el directorio raíz
	if dirPath == "/" {
		// Forzar tipo de directorio para el inodo raíz
		dirInode.I_type[0] = '0'
	} else if dirInode.I_type[0] != '0' {
		return "", fmt.Errorf("la ruta '%s' no es un directorio", dirPath)
	}

	// Generar gráfico DOT
	var dot strings.Builder
	dot.WriteString("digraph G {\n")
	dot.WriteString("  rankdir=\"LR\";\n")
	dot.WriteString("  node [shape=plaintext];\n")
	dot.WriteString("  graph [fontname=\"Arial\", fontsize=10];\n")
	dot.WriteString("  edge [fontname=\"Arial\", fontsize=8];\n\n")

	// Crear tabla principal
	dot.WriteString("  ls_table [label=<\n")
	dot.WriteString("    <table border='0' cellborder='1' cellspacing='0'>\n")
	dot.WriteString("      <tr><td colspan='6' bgcolor='#e0e0e0'><b>Contenido de: " + dirPath + "</b></td></tr>\n")
	dot.WriteString("      <tr><td><b>Permisos</b></td><td><b>Owner</b></td><td><b>Grupo</b></td><td><b>Tamaño</b></td><td><b>Fecha</b></td><td><b>Nombre</b></td></tr>\n")

	// Procesar entradas del directorio
	for i, blockIndex := range dirInode.I_block {
		if blockIndex == -1 || i >= 12 { // Solo bloques directos
			continue
		}

		var folderBlock Partitions.Folderblock
		blockPos := superblock.S_block_start + blockIndex*int32(binary.Size(Partitions.Folderblock{}))
		if err := Utils.LeerArchivo(file, &folderBlock, int64(blockPos)); err != nil {
			continue
		}

		for _, entry := range folderBlock.B_content {
			if entry.B_inodo == -1 {
				continue
			}

			name := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if name == "." || name == ".." {
				continue
			}

			var entryInode Partitions.Inode
			entryInodePos := superblock.S_inode_start + entry.B_inodo*int32(binary.Size(Partitions.Inode{}))
			if err := Utils.LeerArchivo(file, &entryInode, int64(entryInodePos)); err != nil {
				continue
			}

			// Formatear datos
			entryType := "📄"
			if entryInode.I_type[0] == '0' {
				entryType = "📁"
			}

			// Manejo seguro de la fecha
			modTime := "N/A"
			if len(entryInode.I_mtime) >= 4 { // Asegurarse de que haya al menos 4 bytes
				mtimeInt := int64(binary.LittleEndian.Uint32(entryInode.I_mtime[:4]))
				if mtimeInt > 0 {
					modTime = time.Unix(mtimeInt, 0).Format("02/01/2006 ")
				}
			}

			// Añadir fila a la tabla
			dot.WriteString(fmt.Sprintf("      <tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td><td>%s %s</td></tr>\n",
				string(entryInode.I_perm[:]),
				entryInode.I_uid,
				entryInode.I_gid,
				entryInode.I_size,
				modTime,
				entryType,
				name))
		}
	}

	dot.WriteString("    </table>\n")
	dot.WriteString("  >];\n")
	dot.WriteString("}\n")

	return dot.String(), nil
}
