package main

import (
	"fmt"
	"github.com/creatorkostas/progress-bar/pkg"
	"time"
)

func main() {
	/*pb := cmd.NewPBar()
	pb.Total = uint16(10)

	for i := 1; uint16(i) <= pb.Total; i++ {
		pb.Update()
		fmt.Println(i)              // Do something here
		time.Sleep(100000000) // Wait 1 second, for demo purpose
	}

	pb.CleanUp()
	fmt.Println("Done")
	fmt.Println()*/
	pb2 := pkg.NewPBar()
	pb2.Total = uint16(10)

	for i := 1; uint16(i) <= 200; i++ {
		if i == 9 {
			pb2.UpdateTotal(200)
		}
		if i == 30 {
			pb2.UpdateTotal(100)
		}
		pb2.Update()
		fmt.Println(i)       // Do something here
		time.Sleep(50000000) // Wait 1 second, for demo purpose
	}

	pb2.CleanUp()
}
