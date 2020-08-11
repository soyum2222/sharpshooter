
## Sharpshooter


![GitHub](https://img.shields.io/github/license/soyum2222/sharpshooter?logo=Github&style=plastic)  ![GitHub commit activity](https://img.shields.io/github/commit-activity/m/soyum2222/sharpshooter?logo=Github)  <img src="http://visits.myyou.top/sharpshooter/visits" />


Sharpshooter is a reliability network protocol useing UDP.
    
It is a connection-oriented protocol just like TCP.
    
It dont has packet characteristics,You can try it to bypassing some protocol characteristics detection,and base for P2P application transport protocol.
    
About instructions you can see example dir, I provided tow simple example.
    
If want TCP to sharpshooter convert , can try https://github.com/soyum2222/sharpshooterTunel .
    


## Specification

`| SIZE(4byte) | SQE(4byte) | CMD(2byte) | CONTENT(.......) |`


    SIZE:
        contain SQE CMD CONTENT byte size . but not contain itself byte size .
        
    SQE
        sequence number, continuous data package, SQE is continuous.
        
    CMD
        0:ack
        1:NORMAL
        2:first handshack
        3:second handshack(response first handshack)
        4:third handshack
        5:close connction(FIN)
        6:response close
        7:health check
        8:response health 
           


## Use

#### Ping pong

[ping.go](https://github.com/soyum2222/sharpshooter/blob/master/example/ping.go)

[pong.go](https://github.com/soyum2222/sharpshooter/blob/master/example/pong.go)

    
#### File transfer

[send_file.go](https://github.com/soyum2222/sharpshooter/blob/master/example/send_file.go)

[receive_file.go](https://github.com/soyum2222/sharpshooter/blob/master/example/send_file.go)



## Network utilization

try transfer 100M file

![speed](https://github.com/soyum2222/sharpshooter/blob/master/image/network.png)


![utilization](https://github.com/soyum2222/sharpshooter/blob/master/image/network-utilization.png)

utilization depends on network status and send window size

    
