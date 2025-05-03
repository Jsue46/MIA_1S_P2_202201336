package Environment

import (
	"Proyecto/Partitions"
	"Proyecto/Utils"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Estructura para representar espacios libres en el disco
type Space struct {
	Start int32
	Size  int32
}

// Función para encontrar la posición óptima según la estrategia de ajuste
func findPartitionPosition(file *os.File, mbr Partitions.MBR, size int, fit string) (int32, error) {
	diskSize := mbr.MbrTamanio

	// Crear un arreglo para almacenar los espacios disponibles
	var spaces []Space

	// Agregar el espacio inicial (después del MBR)
	mbrSize := int32(binary.Size(mbr))
	var initialSpace Space
	initialSpace.Start = mbrSize
	initialSpace.Size = 0
	spaces = append(spaces, initialSpace)

	// Crear una lista temporal de particiones
	var partitions []Partitions.Partition
	for _, part := range mbr.MbrPartitions {
		if part.PartSize > 0 {
			partitions = append(partitions, part)
		}
	}

	// Ordenar las particiones por posición
	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i].PartStart < partitions[j].PartStart
	})

	// Encontrar espacios disponibles
	currentPos := mbrSize
	for _, part := range partitions {
		if part.PartStart > currentPos {
			// Hay un espacio libre
			var newSpace Space
			newSpace.Start = currentPos
			newSpace.Size = part.PartStart - currentPos
			spaces = append(spaces, newSpace)
		}
		currentPos = part.PartStart + part.PartSize
	}

	// Verificar si hay espacio después de la última partición
	if currentPos < diskSize {
		var lastSpace Space
		lastSpace.Start = currentPos
		lastSpace.Size = diskSize - currentPos
		spaces = append(spaces, lastSpace)
	}

	// Filtrar espacios demasiado pequeños
	var validSpaces []Space
	for _, space := range spaces {
		if space.Size >= int32(size) {
			validSpaces = append(validSpaces, space)
		}
	}

	if len(validSpaces) == 0 {
		return 0, fmt.Errorf("no hay suficiente espacio contiguo para la partición")
	}

	// Aplicar estrategia de ajuste
	var selectedSpace Space

	switch fit {
	case "ff": // First Fit
		selectedSpace = validSpaces[0]
	case "bf": // Best Fit
		selectedSpace = validSpaces[0]
		minExtraSpace := selectedSpace.Size - int32(size)

		for _, space := range validSpaces {
			extraSpace := space.Size - int32(size)
			if extraSpace < minExtraSpace && extraSpace >= 0 {
				selectedSpace = space
				minExtraSpace = extraSpace
			}
		}
	case "wf": // Worst Fit
		selectedSpace = validSpaces[0]
		maxExtraSpace := selectedSpace.Size - int32(size)

		for _, space := range validSpaces {
			extraSpace := space.Size - int32(size)
			if extraSpace > maxExtraSpace {
				selectedSpace = space
				maxExtraSpace = extraSpace
			}
		}
	}

	return selectedSpace.Start, nil
}

func Fdisk(size int, path string, name string, unit string, type_ string, fit string) string {
	var output strings.Builder
	output.WriteString("╔═════════════════════ INICIO  FDISK  ═════════════════════════╗\n")
	output.WriteString(fmt.Sprintf("  Tamaño: %d \n", size))
	output.WriteString(fmt.Sprintf("  Ruta: %s\n", path))
	output.WriteString(fmt.Sprintf("  Nombre: %s\n", name))
	output.WriteString(fmt.Sprintf("  Unidad: %s\n", unit))
	output.WriteString(fmt.Sprintf("  Tipo: %s\n", type_))
	output.WriteString(fmt.Sprintf("  Ajuste: %s\n", fit))

	// Validaciones básicas
	if fit != "bf" && fit != "ff" && fit != "wf" {
		return "Error: El ajuste debe ser 'bf', 'ff' o 'wf'"
	}
	if size <= 0 {
		return "Error: El tamaño debe ser mayor a 0"
	}
	if unit != "b" && unit != "k" && unit != "m" {
		return "Error: La unidad debe ser 'b', 'k' o 'm'"
	}
	if type_ != "p" && type_ != "e" && type_ != "l" {
		return "Error: El tipo debe ser 'p', 'e' o 'l'"
	}
	if name == "" {
		return "Error: El nombre es obligatorio"
	}
	if path == "" {
		return "Error: La ruta es obligatoria"
	}

	// Abrir el archivo binario en la ruta proporcionada
	file, err := Utils.AbrirArchivo(path)
	if err != nil {
		return fmt.Sprintf("  Error: No se pudo abrir el archivo con la ruta proporcionada: %s", path)
	}
	defer file.Close()

	// Leer el MBR
	var TempMBR Partitions.MBR
	if err := Utils.LeerArchivo(file, &TempMBR, 0); err != nil {
		return "Error: No se pudo leer el MBR del archivo"
	}

	// Imprimir el objeto MBR
	output.WriteString(Partitions.ImprimirMBR(TempMBR))
	output.WriteString("───────────────────────────────────────────────────────────────── \n")

	// Validaciones de las particiones
	var primaryCount, extendedCount, totalPartitions int
	var usedSpace int32 = 0
	var extendedIndex int = -1

	for i := 0; i < 4; i++ {
		if TempMBR.MbrPartitions[i].PartSize != 0 {
			totalPartitions++
			usedSpace += TempMBR.MbrPartitions[i].PartSize

			if TempMBR.MbrPartitions[i].PartType[0] == 'p' {
				primaryCount++
			} else if TempMBR.MbrPartitions[i].PartType[0] == 'e' {
				extendedCount++
				extendedIndex = i
			}
		}
	}

	// Validaciones específicas por tipo de partición
	if type_ == "p" || type_ == "e" {
		// Validar que no se exceda el número máximo de particiones primarias y extendidas
		if totalPartitions >= 4 {
			return "Error: No se pueden crear más de 4 particiones primarias o extendidas en total."
		}

		// Validar que solo haya una partición extendida
		if type_ == "e" && extendedCount > 0 {
			return "Error: Solo se permite una partición extendida por disco."
		}

		// Validar que el tamaño de la nueva partición no exceda el tamaño del disco
		if usedSpace+int32(size) > TempMBR.MbrTamanio {
			return "Error: No hay suficiente espacio en el disco para crear esta partición."
		}

		// Obtener posición de inicio basada en la estrategia de ajuste
		start, err := findPartitionPosition(file, TempMBR, size, fit)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		// Verificar si el nombre ya existe
		for i := 0; i < 4; i++ {
			if TempMBR.MbrPartitions[i].PartSize > 0 {
				partName := strings.TrimSpace(string(TempMBR.MbrPartitions[i].PartName[:]))
				if strings.EqualFold(partName, name) {
					return fmt.Sprintf("Error: Ya existe una partición con el nombre '%s'", name)
				}
			}
		}

		// Crear la partición primaria o extendida
		for i := 0; i < 4; i++ {
			if TempMBR.MbrPartitions[i].PartSize == 0 {
				TempMBR.MbrPartitions[i].PartSize = int32(size)
				TempMBR.MbrPartitions[i].PartStart = start
				copy(TempMBR.MbrPartitions[i].PartName[:], name)
				copy(TempMBR.MbrPartitions[i].PartFit[:], fit)
				copy(TempMBR.MbrPartitions[i].PartStatus[:], "0")
				copy(TempMBR.MbrPartitions[i].PartType[:], type_)
				TempMBR.MbrPartitions[i].PartCorrelative = int32(totalPartitions + 1)

				// Si es extendida, inicializar el primer EBR
				if type_ == "e" {
					var ebr Partitions.EBR
					ebr.PartStart = start
					ebr.PartSize = 0
					ebr.PartNext = -1
					ebr.PartFit = fit[0]
					ebr.PartMount = '0'

					// Escribir el EBR inicial
					Utils.EscribirArchivo(file, ebr, int64(start))
				}

				break
			}
		}
	} else if type_ == "l" {
		// Para particiones lógicas, primero comprobar que existe una partición extendida
		if extendedCount == 0 {
			return "Error: No se puede crear una partición lógica sin una partición extendida."
		}

		// Obtener la partición extendida
		extPartition := TempMBR.MbrPartitions[extendedIndex]
		extStart := extPartition.PartStart
		extSize := extPartition.PartSize

		// Verificar si hay espacio suficiente en la partición extendida
		if int32(size) > extSize {
			return "Error: La partición lógica no puede ser mayor que la partición extendida."
		}

		// Leer el primer EBR
		var firstEBR Partitions.EBR
		if err := Utils.LeerArchivo(file, &firstEBR, int64(extStart)); err != nil {
			return "Error: No se pudo leer el EBR inicial de la partición extendida."
		}

		// Verificar si el nombre de la partición lógica ya existe
		// Primero comprobar el primer EBR
		if firstEBR.PartSize > 0 {
			ebrName := strings.TrimSpace(string(firstEBR.PartName[:]))
			if strings.EqualFold(ebrName, name) {
				return fmt.Sprintf("Error: Ya existe una partición lógica con el nombre '%s'", name)
			}
		}

		// Recorrer la lista enlazada de EBRs para verificar nombres duplicados y encontrar el último EBR
		var currentEBR Partitions.EBR = firstEBR
		var currentPos int32 = extStart
		// Eliminar esta línea: var prevPos int32 = extStart

		for currentEBR.PartNext != -1 {
			// Leer el siguiente EBR
			// Eliminar esta línea: prevPos = currentPos
			currentPos = currentEBR.PartNext

			if err := Utils.LeerArchivo(file, &currentEBR, int64(currentPos)); err != nil {
				return "Error: No se pudo leer un EBR en la lista enlazada."
			}

			// Verificar si el nombre ya existe
			if currentEBR.PartSize > 0 {
				ebrName := strings.TrimSpace(string(currentEBR.PartName[:]))
				if strings.EqualFold(ebrName, name) {
					return fmt.Sprintf("Error: Ya existe una partición lógica con el nombre '%s'", name)
				}
			}
		}

		// Ahora que tenemos el último EBR, crear la nueva partición lógica
		if firstEBR.PartSize == 0 && firstEBR.PartNext == -1 {
			// Si el primer EBR está vacío, usarlo
			firstEBR.PartSize = int32(size)
			copy(firstEBR.PartName[:], name)
			firstEBR.PartFit = fit[0]
			firstEBR.PartMount = '0'

			// Escribir el EBR actualizado
			if err := Utils.EscribirArchivo(file, firstEBR, int64(extStart)); err != nil {
				return "Error: No se pudo escribir el primer EBR actualizado."
			}

			output.WriteString(fmt.Sprintf("✅ Partición lógica '%s' creada exitosamente en el primer EBR.\n", name))
		} else {
			// Crear un nuevo EBR al final de la lista
			var newEBRPos int32

			if currentEBR.PartSize == 0 {
				// Si el último EBR está vacío, usarlo
				newEBRPos = currentPos
			} else {
				// Calcular la posición para el nuevo EBR
				newEBRPos = currentPos + int32(binary.Size(currentEBR)) + currentEBR.PartSize

				// Verificar que hay espacio dentro de la partición extendida
				if newEBRPos+int32(size)+int32(binary.Size(currentEBR)) > extStart+extSize {
					return "Error: No hay suficiente espacio en la partición extendida para esta partición lógica."
				}

				// Actualizar el EBR actual para que apunte al nuevo
				currentEBR.PartNext = newEBRPos

				// Escribir el EBR actualizado
				if err := Utils.EscribirArchivo(file, currentEBR, int64(currentPos)); err != nil {
					return "Error: No se pudo actualizar el último EBR."
				}
			}

			// Crear y escribir el nuevo EBR
			var newEBR Partitions.EBR
			newEBR.PartStart = newEBRPos
			newEBR.PartSize = int32(size)
			newEBR.PartNext = -1
			copy(newEBR.PartName[:], name)
			newEBR.PartFit = fit[0]
			newEBR.PartMount = '0'

			if err := Utils.EscribirArchivo(file, newEBR, int64(newEBRPos)); err != nil {
				return "Error: No se pudo escribir el nuevo EBR."
			}

			output.WriteString(fmt.Sprintf("✅ Partición lógica '%s' creada exitosamente en la posición %d.\n", name, newEBRPos))
		}
	}

	// Mostrar las particiones creadas
	output.WriteString("Particiones en el disco:\n")
	for i := 0; i < 4; i++ {
		part := TempMBR.MbrPartitions[i]
		if part.PartSize > 0 {
			// Mostrar información de la partición existente
			output.WriteString(fmt.Sprintf(" Partición %d:\n", i+1))
			output.WriteString(fmt.Sprintf("  Nombre: %s", strings.TrimSpace(string(part.PartName[:]))))
			output.WriteString(fmt.Sprintf(" Tipo: %s", strings.TrimSpace(string(part.PartType[:]))))
			output.WriteString(fmt.Sprintf(" Start: %d", part.PartStart))
			output.WriteString(fmt.Sprintf(" Tamaño: %d", part.PartSize))
			output.WriteString(fmt.Sprintf(" Status: %s", strings.TrimSpace(string(part.PartStatus[:]))))
			output.WriteString(fmt.Sprintf(" Id: %s\n", strings.TrimSpace(string(part.PartID[:]))))
		} else {
			// Mostrar información de una partición vacía con valores predeterminados
			output.WriteString(fmt.Sprintf(" Partición %d:\n", i+1))
			output.WriteString("  Nombre: null")
			output.WriteString(" Tipo: null")
			output.WriteString(" Start: 0")
			output.WriteString(" Tamaño: 0")
			output.WriteString(" Status: null ")
			output.WriteString(" Id: 0\n")
		}
	}

	// Mostrar particiones lógicas si existe una partición extendida
	if extendedIndex != -1 {
		output.WriteString("\nParticiones Lógicas:\n")
		extStart := TempMBR.MbrPartitions[extendedIndex].PartStart

		// Leer el primer EBR
		var currEBR Partitions.EBR
		if err := Utils.LeerArchivo(file, &currEBR, int64(extStart)); err != nil {
			output.WriteString(" Error al leer el primer EBR\n")
		} else {
			// Variable para enumerar las particiones lógicas
			logicalCount := 1

			// Posición actual
			currPos := extStart

			// Recorrer todos los EBRs
			for {
				if currEBR.PartSize > 0 {
					output.WriteString(fmt.Sprintf(" Partición lógica %d:\n", logicalCount))
					output.WriteString(fmt.Sprintf("  Nombre: %s", strings.TrimSpace(string(currEBR.PartName[:]))))
					output.WriteString(fmt.Sprintf(" Start: %d", currEBR.PartStart))
					output.WriteString(fmt.Sprintf(" Tamaño: %d", currEBR.PartSize))
					output.WriteString(fmt.Sprintf(" Next: %d\n", currEBR.PartNext))
					logicalCount++
				}

				if currEBR.PartNext == -1 {
					break
				}

				currPos = currEBR.PartNext
				if err := Utils.LeerArchivo(file, &currEBR, int64(currPos)); err != nil {
					output.WriteString(" Error al leer el siguiente EBR\n")
					break
				}
			}
		}
	}

	// Sobrescribir el MBR
	if err := Utils.EscribirArchivo(file, TempMBR, 0); err != nil {
		return "Error: No se pudo escribir el MBR en el archivo"
	}

	var TempMBR2 Partitions.MBR
	// Leer el objeto nuevamente para verificar
	if err := Utils.LeerArchivo(file, &TempMBR2, 0); err != nil {
		return "Error: No se pudo leer el MBR del archivo después de escribirlo"
	}

	// Imprimir el objeto MBR actualizado
	output.WriteString(Partitions.ImprimirMBR(TempMBR2))
	output.WriteString("╚═════════════════════   FIN   FDISK   ═════════════════════════╝\n")

	return output.String()
}
